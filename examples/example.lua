function lprint(fmt, ...)
    print(string.format(string.format("[LUA] %s", fmt), ...))
end

function doFilter(ezp, pkt) 
    if (pkt.injected) then
        -- This return value is discarded
        return true
    end
    -- Inject some data when data is clientbound
    if pkt.serverbound then
        ezp.inject_to_server(pkt.proxy_id, "Hello from Lua!")
    else 
        ezp.inject_to_client(pkt.proxy_id, "Hello from Lua!")
    end
    -- Drop the original packet
    return false
end


function EzpFilter(ezp, pkt)
    local ok, result = pcall(doFilter, ezp, pkt)
    if not ok then
        lprint("doFilter failed: %s", tostring(result))
        log(LEVEL_WARN, string.format("Filter function failed: %s", tostring(result)))
        -- Allow the packet
        return true
    end
    return result
end

function EzpMain(ezp)
    local server = ezp.get_server_address()
    local proxy = ezp.get_proxy_address()
    while true do
        lprint("Alive=%s", ezp.is_alive())
        lprint("Server=%s | Proxy=%s", server, proxy)
        lprint("BytesSent=%d", ezp.get_bytes_sent())
        lprint("Connected=%d", ezp.get_proxy_count())
        log(LEVEL_INFO, "Hello Lua")
        sleep(10000)
    end
end
