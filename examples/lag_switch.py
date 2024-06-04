#! /usr/bin/env python3
"""
This is a simple lag switch, it can be used for a few different things

1. Create false lag (For instance, modify ping up 500ms) / This only sorta works based on the timeout configured in the proxy
2. Drop all packets (This is typical lag switch stuff)
  3. Drop all incoming packets 
  4. Drop all outgoing packets
5. Create packet loss up to a % (For instance, create 5% packet loss)

This can all be configured with keybinds.
"""
import pynput.keyboard as pyk, yaml, pws_py as pws, time

CONFIG = {
    "Uri": "http://10.0.0.46:8080",
    "ApiKey": "",
    "Keys": {
        # "ping_lag": "kb_f1",
        "drop_incoming": "<ctrl>+<shift>+f",
        "drop_outgoing": "<ctrl>+<shift>+g",
        "drop_all": None,
        # "packet_loss": "kb_f5"
    },
    "Timings": {
        "ping": 200,
        "packet_loss": 10
    }
}

class LagSwitch:
    def __init__(self):
        eps = pws.EzProxyWs(f"{CONFIG['Uri']}", CONFIG["ApiKey"], True, True, True, ls.callback)
        self._ping_lag = False 
        self._drop_incoming = False
        self._drop_outbound = False
        self._packet_loss = False
        hotkeys = {}
        if CONFIG["Keys"]["drop_incoming"] is not None:
            hotkeys[CONFIG["Keys"]["drop_incoming"]] = self._hk_drop_incoming
        if CONFIG["Keys"]["drop_outgoing"] is not None:
            hotkeys[CONFIG["Keys"]["drop_outgoing"]] = self._hk_drop_outbound
        if CONFIG["Keys"]["drop_all"] is not None:
            hotkeys[CONFIG["Keys"]["drop_all"]] = self._hk_drop_both
        with pyk.GlobalHotKeys(hotkeys) as h:
            h.join()
        eps.close()

    def _hk_drop_incoming(self):
        self._drop_incoming = not self._drop_incoming
        if self._drop_incoming:
            print("Dropping incoming packets")
        else:
            print("Allowing incoming packets")


    def _hk_drop_outbound(self):
        self._drop_outbound = not self._drop_outbound
        if self._drop_outbound:
            print("Dropping outbound packets")
        else:
            print("Allowing outbound packets")

    def _hk_drop_both(self):
        if self._drop_incoming or self._drop_outbound:
            self._drop_incoming = False
            self._drop_outbound = False
            print("Allowing all packets")
        else:
            self._drop_incoming = True
            self._drop_outbound = True
            print("Dropping all packets")

    def callback(self, ezp: pws.EzProxyWs, pkt: pws.WsPacket) -> bool:
        if self._drop_incoming and not pkt.Serverbound:
            return False
        if self._drop_outbound and pkt.Serverbound:
            return False
        return True
       
ls = LagSwitch()