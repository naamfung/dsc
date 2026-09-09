local builder = require("builder")

local prompt = builder.new()
    :system("You are a helpful assistant.")
    :user("What is 2+2?")
    :assistant("4")
    :user("And 3+3?")

local messages = prompt:build()
local count: number = prompt:count()

local fork = prompt:clone()
    :user("One more question")

local fork_count: number = fork:count()
local original_count: number = prompt:count()
