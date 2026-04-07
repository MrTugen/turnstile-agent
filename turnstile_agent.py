#!/usr/bin/env python3
import json
import logging
import os
import signal
import sys
import threading
import time
from dataclasses import dataclass
from typing import Optional, List

import requests

try:
    from evdev import InputDevice, categorize, ecodes
except ImportError:
    InputDevice = None
    categorize = None
    ecodes = None

try:
    import RPi.GPIO as GPIO
except ImportError:
    GPIO = None


# -----------------------------
# Config
# -----------------------------
@dataclass
class Config:
    edge_url: str
    edge_api_key: str
    device_name: str
    gpio_pin: int
    relay_active_high: bool
    pulse_ms: int
    scan_cooldown_ms: int
    request_timeout_sec: float
    reader_event_path: str
    reader_name: str
    reader_phys: str
    offline_allow_enabled: bool
    offline_allowlist_path: str
    log_level: str


def load_config() -> Config:
    return Config(
        edge_url=os.getenv("EDGE_URL", "http://127.0.0.1:3000/api/turnstile/scan"),
        edge_api_key=os.getenv("EDGE_API_KEY", ""),
        device_name=os.getenv("DEVICE_NAME", "turnstile-pi-01"),
        gpio_pin=int(os.getenv("GPIO_PIN", "17")),
        relay_active_high=os.getenv("RELAY_ACTIVE_HIGH", "true").lower() == "true",
        pulse_ms=int(os.getenv("PULSE_MS", "300")),
        scan_cooldown_ms=int(os.getenv("SCAN_COOLDOWN_MS", "2000")),
        request_timeout_sec=float(os.getenv("REQUEST_TIMEOUT_SEC", "3.0")),
        reader_event_path=os.getenv("READER_EVENT_PATH", ""),
        reader_name=os.getenv("READER_NAME", "ACS ACR1281 Dual Reader"),
        reader_phys=os.getenv("READER_PHYS", ""),
        offline_allow_enabled=os.getenv("OFFLINE_ALLOW_ENABLED", "false").lower() == "true",
        offline_allowlist_path=os.getenv("OFFLINE_ALLOWLIST_PATH", "/opt/turnstile-agent/allowlist.json"),
        log_level=os.getenv("LOG_LEVEL", "INFO").upper(),
    )


# -----------------------------
# Logging
# -----------------------------
def setup_logging(level: str) -> None:
    logging.basicConfig(
        level=getattr(logging, level, logging.INFO),
        format="%(asctime)s %(levelname)s %(message)s",
    )




# -----------------------------
# Reader discovery
# -----------------------------
def list_input_devices() -> List[InputDevice]:
    if InputDevice is None:
        raise RuntimeError("evdev is not installed")

    devices: List[InputDevice] = []
    base = "/dev/input"
    try:
        names = sorted(os.listdir(base))
    except FileNotFoundError:
        return devices

    for name in names:
        if not name.startswith("event"):
            continue
        path = os.path.join(base, name)
        try:
            devices.append(InputDevice(path))
        except OSError:
            continue
    return devices


def resolve_reader_path(config: Config) -> str:
    if config.reader_event_path:
        if os.path.exists(config.reader_event_path):
            return config.reader_event_path
        raise RuntimeError(f"Configured reader path does not exist: {config.reader_event_path}")

    devices = list_input_devices()
    matches = []
    for dev in devices:
        if config.reader_name and dev.name != config.reader_name:
            continue
        if config.reader_phys and dev.phys != config.reader_phys:
            continue
        matches.append(dev)

    if not matches:
        available = ", ".join(f"{d.path} ({d.name})" for d in devices) or "none"
        raise RuntimeError(
            f"No matching reader found for name={config.reader_name!r} phys={config.reader_phys!r}. "
            f"Available devices: {available}"
        )

    if len(matches) > 1:
        details = ", ".join(f"{d.path} ({d.name}, phys={d.phys})" for d in matches)
        raise RuntimeError(
            "Multiple matching readers found. Set READER_EVENT_PATH or READER_PHYS to disambiguate: "
            + details
        )

    return matches[0].path

# -----------------------------
# Relay
# -----------------------------
class RelayController:
    def __init__(self, gpio_pin: int, active_high: bool):
        self.gpio_pin = gpio_pin
        self.active_high = active_high
        self.enabled = GPIO is not None

        if self.enabled:
            GPIO.setwarnings(False)
            GPIO.setmode(GPIO.BCM)
            GPIO.setup(self.gpio_pin, GPIO.OUT)
            self._set_inactive()
        else:
            logging.warning("RPi.GPIO not available, relay actions will be simulated only")

    def _set_active(self) -> None:
        if self.enabled:
            GPIO.output(self.gpio_pin, GPIO.HIGH if self.active_high else GPIO.LOW)

    def _set_inactive(self) -> None:
        if self.enabled:
            GPIO.output(self.gpio_pin, GPIO.LOW if self.active_high else GPIO.HIGH)

    def pulse(self, pulse_ms: int) -> None:
        logging.info("Triggering relay on GPIO %s for %sms", self.gpio_pin, pulse_ms)
        self._set_active()
        time.sleep(pulse_ms / 1000.0)
        self._set_inactive()

    def cleanup(self) -> None:
        if self.enabled:
            self._set_inactive()
            GPIO.cleanup()


# -----------------------------
# Offline allowlist
# -----------------------------
class OfflineAllowlist:
    def __init__(self, enabled: bool, path: str):
        self.enabled = enabled
        self.path = path
        self.allowed = set()
        self.load()

    def load(self) -> None:
        if not self.enabled:
            return

        try:
            with open(self.path, "r", encoding="utf-8") as f:
                data = json.load(f)
            if isinstance(data, list):
                self.allowed = {self.normalize_uid(x) for x in data if isinstance(x, str)}
                logging.info("Loaded %d offline allowed UIDs", len(self.allowed))
            else:
                logging.warning("Offline allowlist file is not a JSON array")
        except FileNotFoundError:
            logging.warning("Offline allowlist file not found: %s", self.path)
        except Exception as e:
            logging.exception("Failed to load offline allowlist: %s", e)

    @staticmethod
    def normalize_uid(uid: str) -> str:
        return uid.strip().replace(":", "").replace(" ", "").upper()

    def is_allowed(self, uid: str) -> bool:
        if not self.enabled:
            return False
        return self.normalize_uid(uid) in self.allowed


# -----------------------------
# Edge client
# -----------------------------
class EdgeClient:
    def __init__(self, config: Config):
        self.config = config
        self.session = requests.Session()

    @staticmethod
    def normalize_uid(uid: str) -> str:
        return uid.strip().replace(":", "").replace(" ", "").upper()

    def verify_scan(self, uid: str) -> dict:
        normalized_uid = self.normalize_uid(uid)

        payload = {
            "uid": normalized_uid,
            "deviceName": self.config.device_name,
            "timestamp": int(time.time() * 1000),
        }

        headers = {
            "Content-Type": "application/json",
        }

        if self.config.edge_api_key:
            headers["X-API-Key"] = self.config.edge_api_key

        logging.info("Sending UID %s to Edge", normalized_uid)

        response = self.session.post(
            self.config.edge_url,
            json=payload,
            headers=headers,
            timeout=self.config.request_timeout_sec,
        )
        response.raise_for_status()

        data = response.json()
        logging.info("Edge response: %s", data)
        return data


# -----------------------------
# HID keyboard reader
# -----------------------------
KEYMAP = {
    "KEY_0": "0",
    "KEY_1": "1",
    "KEY_2": "2",
    "KEY_3": "3",
    "KEY_4": "4",
    "KEY_5": "5",
    "KEY_6": "6",
    "KEY_7": "7",
    "KEY_8": "8",
    "KEY_9": "9",
    "KEY_A": "A",
    "KEY_B": "B",
    "KEY_C": "C",
    "KEY_D": "D",
    "KEY_E": "E",
    "KEY_F": "F",
}


class HIDCardReader:
    def __init__(self, event_path: str):
        if InputDevice is None:
            raise RuntimeError("evdev is not installed")
        self.event_path = event_path
        self.device = InputDevice(event_path)
        self.buffer = []

    def read_loop(self, callback):
        logging.info("Listening for card scans on %s (%s)", self.event_path, self.device.name)

        for event in self.device.read_loop():
            if event.type != ecodes.EV_KEY:
                continue

            key_event = categorize(event)

            # Only key press, not release/hold
            if key_event.keystate != key_event.key_down:
                continue

            keycode = key_event.keycode
            if isinstance(keycode, list):
                keycode = keycode[0]

            if keycode in ("KEY_ENTER", "KEY_KPENTER"):
                if self.buffer:
                    uid = "".join(self.buffer)
                    self.buffer.clear()
                    callback(uid)
                continue

            char = KEYMAP.get(keycode)
            if char:
                self.buffer.append(char)


# -----------------------------
# Main agent
# -----------------------------
class TurnstileAgent:
    def __init__(self, config: Config):
        self.config = config
        self.relay = RelayController(config.gpio_pin, config.relay_active_high)
        self.edge = EdgeClient(config)
        self.allowlist = OfflineAllowlist(config.offline_allow_enabled, config.offline_allowlist_path)
        self.running = True

        self.last_uid: Optional[str] = None
        self.last_scan_ts_ms: int = 0
        self.lock = threading.Lock()

    @staticmethod
    def normalize_uid(uid: str) -> str:
        return uid.strip().replace(":", "").replace(" ", "").upper()

    def should_ignore_scan(self, uid: str) -> bool:
        now = int(time.time() * 1000)
        uid = self.normalize_uid(uid)

        with self.lock:
            if self.last_uid == uid and (now - self.last_scan_ts_ms) < self.config.scan_cooldown_ms:
                return True

            self.last_uid = uid
            self.last_scan_ts_ms = now
            return False

    def handle_scan(self, raw_uid: str) -> None:
        uid = self.normalize_uid(raw_uid)
        if not uid:
            return

        if self.should_ignore_scan(uid):
            logging.info("Ignoring duplicate scan: %s", uid)
            return

        logging.info("Received UID: %s", uid)

        granted = False
        reason = "unknown"

        try:
            data = self.edge.verify_scan(uid)
            granted = bool(
                data.get("granted")
                or data.get("access")
                or data.get("allowed")
            )
            reason = str(data.get("reason", "edge_response"))
        except Exception as e:
            logging.exception("Edge verification failed: %s", e)
            if self.allowlist.is_allowed(uid):
                granted = True
                reason = "offline_allowlist"
            else:
                granted = False
                reason = "edge_error"

        if granted:
            logging.info("Access granted for %s (%s)", uid, reason)
            self.relay.pulse(self.config.pulse_ms)
        else:
            logging.warning("Access denied for %s (%s)", uid, reason)

    def run(self) -> None:
        resolved_path = resolve_reader_path(self.config)
        logging.info("Resolved reader path: %s", resolved_path)
        reader = HIDCardReader(resolved_path)
        reader.read_loop(self.handle_scan)

    def shutdown(self) -> None:
        logging.info("Shutting down agent")
        self.running = False
        self.relay.cleanup()


# -----------------------------
# Entrypoint
# -----------------------------
agent: Optional[TurnstileAgent] = None


def handle_signal(signum, frame):
    global agent
    logging.info("Signal received: %s", signum)
    if agent:
        agent.shutdown()
    sys.exit(0)


def main():
    global agent
    config = load_config()
    setup_logging(config.log_level)

    signal.signal(signal.SIGINT, handle_signal)
    signal.signal(signal.SIGTERM, handle_signal)

    logging.info("Starting Turnstile Agent")
    logging.info("Device name: %s", config.device_name)
    logging.info("Edge URL: %s", config.edge_url)
    logging.info("GPIO pin: %s", config.gpio_pin)
    logging.info("Configured reader event path: %s", config.reader_event_path or "<auto>")
    logging.info("Configured reader name: %s", config.reader_name or "<none>")
    logging.info("Configured reader phys: %s", config.reader_phys or "<none>")

    agent = TurnstileAgent(config)
    agent.run()


if __name__ == "__main__":
    main()