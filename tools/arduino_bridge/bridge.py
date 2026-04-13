#!/usr/bin/env python3
"""
arduino_bridge.py — Bridge between Arduino Uno (micro-HLL) and Go agent/aggregator.

Dual purpose:
  1. Sends identical IP streams to Arduino (serial) and Go agent (gRPC InjectIPs)
  2. Collects and compares cardinality estimates from both
  3. Forwards Arduino FWD: lines to the Go agent (sensor mode)

Usage:
  # Comparison benchmark (send random IPs to both, compare estimates)
  python3 bridge.py --mode benchmark --serial /dev/ttyACM0 --agent localhost:50052

  # Sensor bridge (forward Arduino FWD: lines to Go agent)
  python3 bridge.py --mode sensor --serial /dev/ttyACM0 --agent localhost:50052

  # Arduino-only test (no Go agent needed)
  python3 bridge.py --mode arduino-only --serial /dev/ttyACM0

  # Simulated (no Arduino hardware, test the protocol)
  python3 bridge.py --mode benchmark --serial fake --agent localhost:50052

Requirements:
  pip install pyserial grpcio grpcio-tools
"""

import argparse
import random
import sys
import time
import struct

# ─── Optional imports (graceful degradation) ─────────────────────────

try:
    import serial
    HAS_SERIAL = True
except ImportError:
    HAS_SERIAL = False

try:
    import grpc
    HAS_GRPC = True
except ImportError:
    HAS_GRPC = False


# ─── Fake serial for testing without hardware ────────────────────────

class FakeSerial:
    """Simulates Arduino responses for testing without hardware."""

    def __init__(self):
        self._buf = b""
        self._registers = [0] * 16  # p=4
        self._count = 0

    def write(self, data):
        line = data.decode().strip()
        if line.startswith("INSERT "):
            self._count += 1
            self._buf += b"OK\n"
        elif line == "ESTIMATE":
            # Very rough fake estimate
            est = min(self._count, int(self._count * 0.8 + random.randint(0, 5)))
            self._buf += f"EST:{est}\n".encode()
        elif line == "RESET":
            self._count = 0
            self._buf += b"OK\n"
        elif line == "MEMINFO":
            self._buf += b"MEM:12\n"
        elif line == "STATS":
            self._buf += f"INSERTS:{self._count}\nEST:{self._count}\nMODE:DUAL\nP:4\nM:16\nREG_BYTES:12\n".encode()
        elif line == "EXPORT":
            self._buf += b"REG:000000000000000000000000\n"
        else:
            self._buf += b"OK\n"

    def readline(self):
        if b"\n" in self._buf:
            line, self._buf = self._buf.split(b"\n", 1)
            return line + b"\n"
        return b""

    @property
    def in_waiting(self):
        return len(self._buf)

    def close(self):
        pass


# ─── Arduino communication ───────────────────────────────────────────

class ArduinoHLL:
    """Communicates with Arduino running micro_hll.ino over serial."""

    def __init__(self, port, baud=115200, timeout=2):
        if port == "fake":
            self.ser = FakeSerial()
            self.fake = True
        else:
            if not HAS_SERIAL:
                print("ERROR: pyserial not installed. Run: pip install pyserial")
                sys.exit(1)
            self.ser = serial.Serial(port, baud, timeout=timeout)
            self.fake = False
            time.sleep(2)  # Wait for Arduino reset
            # Drain startup messages
            while self.ser.in_waiting:
                self.ser.readline()

    def send_command(self, cmd):
        """Send a command and return all response lines."""
        self.ser.write((cmd + "\n").encode())
        time.sleep(0.01)  # Give Arduino time to process
        lines = []
        deadline = time.time() + 1.0  # 1s timeout
        while time.time() < deadline:
            if self.fake:
                line = self.ser.readline()
            else:
                line = self.ser.readline()
            if line:
                decoded = line.decode().strip()
                if decoded:
                    lines.append(decoded)
                    # If we got a terminal response, stop waiting
                    if decoded.startswith(("OK", "EST:", "MEM:", "REG:", "ERR:")):
                        break
            else:
                break
        return lines

    def insert(self, ip):
        return self.send_command(f"INSERT {ip}")

    def estimate(self):
        lines = self.send_command("ESTIMATE")
        for line in lines:
            if line.startswith("EST:"):
                return int(line[4:])
        return -1

    def reset(self):
        self.send_command("RESET")

    def meminfo(self):
        lines = self.send_command("MEMINFO")
        for line in lines:
            if line.startswith("MEM:"):
                return int(line[4:])
        return -1

    def export_registers(self):
        lines = self.send_command("EXPORT")
        for line in lines:
            if line.startswith("REG:"):
                return line[4:]
        return ""

    def stats(self):
        return self.send_command("STATS")

    def set_mode(self, mode):
        return self.send_command(f"MODE {mode}")

    def close(self):
        self.ser.close()


# ─── gRPC agent communication ────────────────────────────────────────

class AgentClient:
    """Communicates with the Go agent via gRPC InjectIPs."""

    def __init__(self, addr):
        if not HAS_GRPC:
            print("ERROR: grpcio not installed. Run: pip install grpcio grpcio-tools")
            sys.exit(1)
        self.channel = grpc.insecure_channel(addr)
        # We need the generated stubs — try to import them
        try:
            sys.path.insert(0, ".")
            import server.server_pb2 as pb2
            import server.server_pb2_grpc as pb2_grpc
            self.pb2 = pb2
            self.stub = pb2_grpc.HllServiceStub(self.channel)
        except ImportError:
            # Generate stubs on the fly or use raw proto
            print("WARNING: gRPC stubs not found. Run 'make proto' first.")
            print("         Agent comparison will be skipped.")
            self.stub = None

    def inject_ips(self, ips, byte_len=64):
        if self.stub is None:
            return
        req = self.pb2.InjectIPsBatchRequest(ips=ips, byte_len=byte_len)
        self.stub.InjectIPs(req)

    def get_estimate(self):
        if self.stub is None:
            return -1
        resp = self.stub.GetEstimate(self.pb2.GetEstimateRequest())
        return resp.estimate

    def close(self):
        self.channel.close()


# ─── IP generation ────────────────────────────────────────────────────

def generate_ips(n, seed=42):
    """Generate n unique random IPv4 addresses."""
    rng = random.Random(seed)
    ips = set()
    while len(ips) < n:
        ip = f"{rng.randint(1,223)}.{rng.randint(0,255)}.{rng.randint(0,255)}.{rng.randint(1,254)}"
        ips.add(ip)
    return list(ips)


# ─── Benchmark mode ──────────────────────────────────────────────────

def run_benchmark(arduino, agent, counts, seed=42):
    """Send known IP counts to both Arduino and agent, compare estimates."""

    print("=" * 72)
    print(f"{'Distinct IPs':>12} | {'True':>8} | {'Arduino p=4':>12} | {'Err%':>6} | {'Agent p=14':>12} | {'Err%':>6}")
    print("-" * 72)

    results = []

    for n in counts:
        arduino.reset()
        ips = generate_ips(n, seed=seed + n)

        # Send to Arduino (one at a time over serial — slow but accurate)
        t0 = time.time()
        for ip in ips:
            arduino.insert(ip)
        arduino_time = time.time() - t0

        arduino_est = arduino.estimate()
        arduino_err = abs(arduino_est - n) / n * 100 if n > 0 else 0

        # Send to agent (batch via gRPC — fast)
        agent_est = -1
        agent_err = -1
        if agent:
            try:
                # Send in batches of 500
                for i in range(0, len(ips), 500):
                    batch = ips[i:i+500]
                    agent.inject_ips(batch)
                time.sleep(0.5)  # Let agent process
                agent_est = agent.get_estimate()
                agent_err = abs(agent_est - n) / n * 100 if n > 0 else 0
            except Exception as e:
                agent_est = -1
                agent_err = -1

        agent_str = f"{agent_est:>12}" if agent_est >= 0 else "      N/A   "
        agent_err_str = f"{agent_err:>5.1f}%" if agent_err >= 0 else "  N/A "

        print(f"{n:>12,} | {n:>8,} | {arduino_est:>12,} | {arduino_err:>5.1f}% | {agent_str} | {agent_err_str}")

        results.append({
            "true_count": n,
            "arduino_estimate": arduino_est,
            "arduino_error_pct": round(arduino_err, 2),
            "arduino_time_s": round(arduino_time, 2),
            "agent_estimate": agent_est,
            "agent_error_pct": round(agent_err, 2) if agent_err >= 0 else None,
        })

    print("=" * 72)
    arduino_mem = arduino.meminfo()
    print(f"\nArduino HLL memory: {arduino_mem} bytes (p={4}, m={16})")
    if agent:
        print(f"Go agent HLL memory: ~12,288 bytes (p=14, m=16384)")
    print(f"Memory ratio: {12288 / arduino_mem:.0f}× more memory → "
          f"{26.0:.1f}% vs 0.81% standard error")

    return results


# ─── Sensor bridge mode ──────────────────────────────────────────────

def run_sensor_bridge(arduino, agent):
    """Arduino in SENSOR mode: forward FWD: lines to Go agent."""
    if not agent:
        print("ERROR: Agent address required for sensor mode.")
        return

    arduino.set_mode("SENSOR")
    print("Sensor bridge active. Arduino → Pi/Agent forwarding.")
    print("Press Ctrl+C to stop.\n")

    batch = []
    batch_size = 50
    forwarded = 0

    try:
        while True:
            if arduino.fake:
                time.sleep(1)
                continue
            if arduino.ser.in_waiting:
                line = arduino.ser.readline().decode().strip()
                if line.startswith("FWD:"):
                    ip = line[4:]
                    batch.append(ip)
                    if len(batch) >= batch_size:
                        agent.inject_ips(batch)
                        forwarded += len(batch)
                        batch.clear()
                        print(f"\rForwarded: {forwarded} IPs", end="", flush=True)
            else:
                # Flush partial batch
                if batch:
                    agent.inject_ips(batch)
                    forwarded += len(batch)
                    batch.clear()
                time.sleep(0.01)
    except KeyboardInterrupt:
        if batch:
            agent.inject_ips(batch)
            forwarded += len(batch)
        print(f"\nTotal forwarded: {forwarded} IPs")


# ─── Arduino-only test mode ──────────────────────────────────────────

def run_arduino_only(arduino, counts, seed=42):
    """Test Arduino micro-HLL without any Go agent."""

    print("=" * 56)
    print(f"{'Distinct IPs':>12} | {'True':>8} | {'Arduino p=4':>12} | {'Error%':>8}")
    print("-" * 56)

    for n in counts:
        arduino.reset()
        ips = generate_ips(n, seed=seed + n)

        t0 = time.time()
        for ip in ips:
            arduino.insert(ip)
        elapsed = time.time() - t0

        est = arduino.estimate()
        err = abs(est - n) / n * 100 if n > 0 else 0

        print(f"{n:>12,} | {n:>8,} | {est:>12,} | {err:>6.1f}%  ({elapsed:.1f}s)")

    print("=" * 56)
    print(f"\nArduino stats:")
    for line in arduino.stats():
        print(f"  {line}")
    print(f"\nExported registers: {arduino.export_registers()}")


# ─── Main ─────────────────────────────────────────────────────────────

def main():
    parser = argparse.ArgumentParser(
        description="Arduino micro-HLL bridge and comparison tool")
    parser.add_argument("--mode", choices=["benchmark", "sensor", "arduino-only"],
                        default="benchmark", help="Operating mode")
    parser.add_argument("--serial", default="/dev/ttyACM0",
                        help="Serial port for Arduino (use 'fake' for simulation)")
    parser.add_argument("--baud", type=int, default=115200, help="Serial baud rate")
    parser.add_argument("--agent", default="", help="Go agent gRPC address (host:port)")
    parser.add_argument("--counts", default="10,50,100,500,1000",
                        help="Comma-separated distinct IP counts to test")
    parser.add_argument("--seed", type=int, default=42, help="RNG seed")
    args = parser.parse_args()

    counts = [int(x) for x in args.counts.split(",")]

    print(f"Arduino serial: {args.serial} @ {args.baud} baud")
    arduino = ArduinoHLL(args.serial, args.baud)

    agent = None
    if args.agent and args.mode != "arduino-only":
        print(f"Go agent: {args.agent}")
        agent = AgentClient(args.agent)

    try:
        if args.mode == "benchmark":
            run_benchmark(arduino, agent, counts, args.seed)
        elif args.mode == "sensor":
            run_sensor_bridge(arduino, agent)
        elif args.mode == "arduino-only":
            run_arduino_only(arduino, counts, args.seed)
    finally:
        arduino.close()
        if agent:
            agent.close()


if __name__ == "__main__":
    main()
