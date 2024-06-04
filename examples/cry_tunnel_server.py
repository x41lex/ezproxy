#! /usr/bin/env python3
"""
Encryption tunnel server

I would not use this in production, or for any real security purpose, its just a interesting idea I had.
"""
import pws_py as pws, typing, threading, time
from Crypto.Cipher import AES, PKCS1_OAEP
from Crypto.PublicKey import RSA 
TARGET = "10.0.0.46:5556"
API_KEY = "BABE"
TARGET_HTTP = f"http://{TARGET}"
TARGET_WS = f"ws://{TARGET}"

class Client:
    def __init__(self, px: pws.EzProxyWs, client: str, server: str, proxyNum: int):
        pass

    def _encrypt_rsa(self, data: bytes) -> bytes:
        pass

    def _encrypt_aes(self, data: bytes) -> bytes:
        pass

    def _decrypt_aes(self, data: bytes) -> bytes:
        pass

    def _send_data(self, data: bytes, serverbound: bool, etype: typing.Literal["rsa", "aes"]):
        pass

    def _set_rsa_key(self, key: bytes):
        pass

    def _handle_client_pkt(self, px: pws.WsPacket) -> bool:
        pass

    def _handle_server_pkt(self, px: pws.WsPacket) -> bool:
        pass

    def IsSetup(self) -> bool:
        pass

    def Close(self):
        pass 

    def IsAlive(self) -> bool:
        pass

    def HandlePacket(self, px: pws.WsPacket) -> bool:
        pass

class Central:
    def __init__(self, api_addr: str, api_key: bool):
        self._api_addr = api_addr
        self._api_key = api_key
        self._pws = pws.ApiClient(f"http://{self._api_addr}", self._api_key)
        self._clients: typing.Dict[int, Client] = {}
        self._alive = True
        self._pruner_thread = threading.Thread(target=self._pruner, daemon=True)
        self._pruner_thread.start()
        
    def _pruner(self):
        # Seconds we've waited to prevent locking with _alive
        cnt = 30
        while self._alive:
            if cnt < 30:
                time.sleep(1)
                cnt += 1
                continue
            cnt = 0
            proxies = self._pws.proxyList()
            # Any proxy not removed is deleted.
            dead = list(self._clients.keys())
            for x in proxies:
                if x.Alive and x.Id in dead:
                    dead.remove(x.Id)
            for x in dead:
                del self._clients[x]

    def _callback(self, px: pws.EzProxyWs, pkt: pws.WsPacket) -> bool:
        # Return if we should allow the packet
        # Ignore non TCP
        if pkt.Network != "tcp":
            return False
        if pkt.ProxyId not in self._clients:
            # Setup the client
            self._clients[pkt.ProxyId] = Client(px, pkt.Client, pkt.Server, pkt.ProxyId)
        if self._clients[pkt.ProxyId].IsAlive():
            try:
                self._clients[pkt.ProxyId].HandlePacket(pkt)
            except Exception as e:
                print(f"[FATAL] Client error, closing client: {e}")
                self._clients[pkt.ProxyId].Close()
                del self._clients[pkt.ProxyId]
        else:
            del self._clients[pkt.ProxyId]
        return False
        
    def Close(self):
        self._alive = False

    def IsAlive(self):
        return self._alive

    def Join(self, timeout: float = None):
        self._pruner_thread.join(timeout)