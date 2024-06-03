#! /usr/bin/env python3
import pws_py as pws, time
TARGET = "10.0.0.46:8080"
TARGET_HTTP = f"http://{TARGET}"
TARGET_WS = f"ws://{TARGET}"
API_KEY = "BABE"

SHOULD_STOP = True

def print_dict(d: dict, prefix=""):
    if hasattr(d, "__dict__"):
        d = d.__dict__
    max_key = max([len(x) for x in d.keys()])
    for k, v in d.items():
        k = str(k)
        print(f"{prefix}{k.ljust(max_key)}: {v}")

def pkt_callback(px: pws.EzProxyWs, pkt: pws.WsPacket) -> bool:
    # Don't get injected packets
    print(f"[{pkt.Source} => {pkt.Dest}{' (Injected)' if pkt.Flags & pws.CapFlag_Injected != 0 else ''}] {{{pkt.PktNum}}} {pkt.Data}")
    if pkt.Flags & pws.CapFlag_Injected != 0:
        # Can't filter injected packets
        return True
    if pkt.Serverbound:
        #print(f"Allow (To Server)")
        return True
    #px.send_to_client(-1, b"From Client")
    #print(f"Drop (From Server)")
    return True
    #if pkt.Data == b"GOODBYE WORLD":
    #    global SHOULD_STOP
    #    SHOULD_STOP = False
    #    print(f"Got GOODBYE packet sending response.")
    #    px.send_to_client(pkt.ProxyId, b"FUCK YOU")
    #    return False

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
    ws = pws.EzProxyWs(TARGET_WS, API_KEY, can_close=False, can_filter=True, can_inject=True, callback=pkt_callback)
    try:
        while SHOULD_STOP:
            time.sleep(1)
    except KeyboardInterrupt:
        ws.close()
    except Exception as e:
        ws.close()
        raise

main()