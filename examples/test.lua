function lprint(fmt, ...)
    print(string.format(string.format("[LUA] %s", fmt), ...))
end

function EzpFilter(ezp, pkt)
    if pkt.injected then 
        return true
    end

    
end
