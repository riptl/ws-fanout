# WebSocket Fanout

`PORT=8001 WS_SOURCE=ws://example.org:8002 ./ws-fanout`

 - Connects to `ws://example.org:8002` (source)
 - Listens on `:8001` (sinks)
 - Forwards every message from source to each connected peer
