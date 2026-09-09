local json = {}

function json.encode(value: any): string
    return "encoded"
end

function json.decode(source: string): (any, string?)
    return {
        data = "test",
        modelVersion = "gemini-2.5-pro-001",
        responseId = "resp-123",
        createTime = "2024-01-15T10:30:00Z",
    }, nil
end

return json
