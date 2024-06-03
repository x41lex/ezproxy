"""
API requests
"""
import requests, dataclasses, typing, json, base64
from .ws import EzProxyWs

@dataclasses.dataclass
class ProxyStatus:
    """
    Status of a proxy
    """
    Id: int 
    Alive: bool
    Address: str
    Network: str
    BytesSent: int
    LastContactAgo: float

@dataclasses.dataclass
class SpawnerStatus:
    """
    Status of the spawner
    """
    ConnectionCount: int
    Alive: bool
    BytesSent: int
    ProxyAddress: str
    ServerAddress: str

@dataclasses.dataclass
class NewKey:
    """
    New key data
    """
    Key: str
    Perms: int

@dataclasses.dataclass
class KeyInfo:
    """
    Info about a key
    """
    Value: int
    CanCheckStatus: bool
    CanClose: bool
    CanUseWebsocket: bool
    CanFilter: bool
    CanInject: bool
    CanMakeKeys: bool
    CanDuplicateKeys: bool
    Admin: bool

class ApiClient:
    def __init__(self, uri: str, key: str):
        self._api_key = key
        self._uri = uri

    def _do_request(self, method: str, uri: str, data:bytes=None) -> any:
        r = requests.request(method, uri, data=data)
        if r.status_code != 200:
            # Web error
            r.raise_for_status()
            raise RuntimeError("expected a requests error")
        jdata = r.json()
        if jdata["Status"] == 401:
            # Unauthorized - missing or invalid API key
            raise PermissionError(f"api returned unauthorized: {jdata['Data']}")
        elif jdata["Status"] == 403:
            raise PermissionError(f"api key lacks permission for this request: {jdata['Data']}")
        elif jdata["Status"] != 200:
            raise ValueError(f"api returned status code {jdata['Data']}")
        return jdata["Data"]

    def _get(self, endpoint: str, version: int, queries: dict = {}, with_key=True) -> any:
        ep = f"{self._uri}/api/{version}/{endpoint}?"
        if with_key:
            queries["key"] = self._api_key
        for k, v in queries.items():
            ep += f"{k}={v}&"
        ep = ep[:-1]
        return self._do_request("get", ep)
    
    def _post(self, endpoint: str, version: int, data: bytes, queries: dict = {}, with_key=True) -> any:
        ep = f"{self._uri}/api/{version}/{endpoint}?"
        if with_key:
            queries["key"] = self._api_key
        for k, v in queries.items():
            ep += f"{k}={v}&"
        ep = ep[:-1]
        r = requests.post(ep, data)
        if r.status_code != 200:
            r.raise_for_status()
            raise RuntimeError("expected a requests error")
        jdata = r.json()
        if jdata["Status"] != 200:
            raise ValueError(f"api returned error {jdata['Status']}")
        return jdata["Data"]
    
    def spawnerStatus(self) -> SpawnerStatus:
        data = self._get("status", 1, with_key=True)
        return SpawnerStatus(**data)
    
    def injectData(self, target: int, data: bytes, to_client: bool, to_server: bool) -> None:
        if not to_client and not to_server:
            raise ValueError("to_client and/or to_server must be true")
        post_data = json.dumps({
            "Id": target,
            "Data": base64.b64encode(data).decode(),
            "ToClient": to_client,
            "ToServer": to_server
        }).encode()
        self._post("inject", 1, post_data, with_key=True)
    
    def proxyList(self) -> typing.List[ProxyStatus]:
        data = self._get("proxies", 1, with_key=True)
        return [ProxyStatus(**x) for x in data]

    def newKey(self, permissions: int) -> NewKey:
        data = self._get("newkey", 1, {"perms": permissions}, with_key=True)
        return NewKey(**data)
    
    def keyInfo(self, key: str=None) -> KeyInfo:
        data = self._get("keyinfo", 1, {"key": key if key is not None else self._api_key}, with_key=False)
        return KeyInfo(**data)
    