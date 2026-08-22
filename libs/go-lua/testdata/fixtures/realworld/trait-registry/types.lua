type TraitToolDef = string | {
    id: string,
    context: {[string]: any}?,
    description: string?,
    alias: string?,
}

type TraitToolEntry = {
    id: string,
    context: {[string]: any}?,
    description: string?,
    alias: string?,
}

type TraitSpec = {
    id: string,
    name: string,
    description: string,
    prompt: string,
    tools: {TraitToolEntry},
    context: {[string]: any},
}

type TraitRegistryEntry = {
    id: string,
    meta: {type: string?, name: string?, comment: string?}?,
    data: {
        prompt: string?,
        tools: {TraitToolDef}?,
        context: {[string]: any}?,
    }?,
}

local M = {}
M.TraitToolDef = TraitToolDef
M.TraitToolEntry = TraitToolEntry
M.TraitSpec = TraitSpec
M.TraitRegistryEntry = TraitRegistryEntry

M.TRAIT_TYPE = "agent.trait"

return M
