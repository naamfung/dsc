local function printf(fmt: string, ...: any)
    print(string.format(fmt, ...))
end
printf("Hello %s", "world")
