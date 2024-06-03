"""
Websocket bindings
"""
from __future__ import annotations
from websockets.sync.client import connect as ws_connect, ClientConnection
import typing, dataclasses, enum, json, base64, threading, websockets
from .const import *

class WsReqType(enum.Enum):
    Inject = 1
    Close = 2
    Filter = 3

class InjectDirection(enum.Enum):
    Client = 1 << 0
    Server = 1 << 1
    Both = 1 << 0 | 1 << 1

class WsServerType(enum.Enum):
    Error = -1 # Should never be given
    Packet = 1

@dataclasses.dataclass
class WsPacket:
    """
    Captured packet
    """
    PktNum:  int
    Network: str
    Source:  str
    ProxyId: int
    Dest:    str
    Data:    bytes
    Flags:   int

    @property
    def Serverbound(self) -> bool:
        return self.Flags&1 != 0
    
    @property
    def Server(self) -> str:
        if self.Serverbound:
            return self.Dest
        return self.Source
    
    @property
    def Client(self) -> str:
        if not self.Serverbound:
            return self.Dest
        return self.Source

@dataclasses.dataclass
class WsServerMsg:
    Status: int
    Type: int
    Data: typing.Union[WsPacket, int]


def WsServerFromJson(jData: str) -> WsServerMsg:
    """
    Create a WsServer instance from JSON data
    """
    # Data is formatted as 
    # Status => int
    # Data => dict|str
    #   Type => int
    #   Data => dict
    data = json.loads(jData)
    # Status error
    if data["Status"] != 200:
        # Data is a error msg
        return WsServerMsg(data["Status"], 0, data["Data"])
    # Strip the status - its 200
    data = data["Data"]
    if data["Type"] == WsServerType.Error.value:
        raise ValueError(f"got type -1 from websocket")
    elif data["Type"] == WsServerType.Packet.value:
        inner = data["Data"]
        pkt = WsPacket(inner["PktNum"], inner["Network"], inner["Source"], inner["ProxyId"], inner["Dest"], base64.b64decode(inner["Data"].encode()), inner["Flags"])
        return WsServerMsg(200, data["Type"], pkt)
    else:
        raise NotImplementedError(f"unknown data type {data['Type']}")         
    
@dataclasses.dataclass
class WsClientMsg:
    Type: WsReqType
    Target: int
    Data: bytes
    Extra: int

    def to_json(self) -> str:
        return json.dumps({
            "Type": self.Type.value,
            "Target": self.Target,
            "Data": base64.b64encode(self.Data).decode(),
            "Extra": self.Extra
        })

class EzProxyWs:
    def __init__(self, uri: str, key: str, can_filter: bool, can_inject: bool, can_close: bool, callback:typing.Callable[[EzProxyWs, WsPacket], bool]=None):
        uri += f"/api/2/socket?key={key}&"
        if can_filter:
            uri += "filter&"
        if can_inject:
            uri += "inject&"
        if can_close:
            uri += "close&"
        uri = uri[:-1]
        self._con = ws_connect(uri)
        self._alive = True
        self._pkt_callback = callback
        self._can_filter = can_filter
        self._can_close = can_close
        self._can_inject = can_inject
        self._thread = threading.Thread(target=self._listen)
        self._thread.start()

    def _listen(self):
        while self._alive:
            try:
                data = self._con.recv(2)
            except TimeoutError:
                continue
            except websockets.exceptions.ConnectionClosedOK:
                return
            if isinstance(data, bytes):
                # Dont handle binary
                raise ValueError(f"Got binary data from WebSocket, dont know how to handle")
            try:
                sv = WsServerFromJson(data)
            except NotImplementedError as e:
                self.close()
                raise
            if sv.Status != 200:
                if sv.Status == 408:
                    # Timed out - find some way to handle this better.
                    continue
                raise ValueError(f"WebSocket error: {sv.Status}, {sv.Data}")
            if self._pkt_callback is not None and sv.Type == WsServerType.Packet.value:
                r = self._pkt_callback(self, sv.Data)
                # We can only filter is we have permission & the packet is not injected
                if self._can_filter and sv.Data.Flags & CapFlag_Injected == 0:
                    self.filter(sv.Data.PktNum, r)
                

    def filter(self, pktId: int, allow: bool):
        msg = WsClientMsg(WsReqType.Filter, pktId, b"", 1 if allow else 0)
        self._con.send(msg.to_json())

    def inject(self, target: int, data: bytes, direction: typing.Literal[InjectDirection.Client, InjectDirection.Server, InjectDirection.Both]):
        msg = WsClientMsg(WsReqType.Inject, target, data, direction.value)
        self._con.send(msg.to_json())

    def send_to_server(self, target: int, data: bytes):
        self.inject(target, data, InjectDirection.Server)

    def send_to_client(self, target: int, data: bytes):
        self.inject(target, data, InjectDirection.Client)

    def close_proxy(self, target: int):
        msg = WsClientMsg(WsReqType.Close, target, b"", 0)
        self._con.send(msg.to_json())

    def close(self):
        self._alive = False
        self._con.close()
    
    def set_pkt_callback(self, cb: typing.Callable[[EzProxyWs, WsPacket], bool]):
        self._pkt_callback = cb

    def is_alive(self) -> bool:
        return self._alive
    
    def join(self, timeout: typing.Union[float, None]=None):
        self._thread.join(timeout)
