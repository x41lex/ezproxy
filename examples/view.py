#! /usr/bin/env python3
"""
View packets from the Proxy

Modify 'TARGET' and 'API_KEY' to setup.
"""
import pws_py as pws, time
TARGET = "10.0.0.46:8080"
API_KEY = "BABE"
TARGET_HTTP = f"http://{TARGET}"
TARGET_WS = f"ws://{TARGET}"

SHOULD_STOP = True

WITH_FILTER = True

def print_dict(d: dict, prefix=""):
    if hasattr(d, "__dict__"):
        d = d.__dict__
    max_key = max([len(x) for x in d.keys()])
    for k, v in d.items():
        k = str(k)
        print(f"{prefix}{k.ljust(max_key)}: {v}")

CNT = 0

def pkt_callback(px: pws.EzProxyWs, pkt: pws.WsPacket) -> bool:
    # Don't get injected packets
    print(f"[{pkt.Source} => {pkt.Dest}{' (Injected)' if pkt.Flags & pws.CapFlag_Injected != 0 else ''}] {{{pkt.PktNum}}} {pkt.Data}")
    global CNT 
    if CNT >= 3:
        CNT = 0
        return False
    return True

def main():
    cl = pws.ApiClient(TARGET_HTTP, API_KEY)
    ki = cl.keyInfo()
    print(f"* Key Info")
    print_dict(ki, prefix="|  ")
    ss = cl.spawnerStatus()
    print(f"* SpawnerStatus")
    print_dict(ss, prefix="|  ")
    pi = cl.proxyList()
    print(f"* ProxyList ({len(pi)} elements)")
    for i, x in enumerate(pi):
        print(f"| [{i}]")
        print_dict(x, prefix="  |  ")
    ws = pws.EzProxyWs(TARGET_WS, API_KEY, can_close=False, can_filter=WITH_FILTER, can_inject=True, callback=pkt_callback)
    try:
        while SHOULD_STOP:
            time.sleep(1)
    except KeyboardInterrupt:
        ws.close()
    except Exception as e:
        ws.close()
        raise

main()