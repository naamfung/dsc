local json = require("json")
local uuid = require("uuid")
local expr = require("expr")
local consts = require("df_consts")

local compiler = {}

compiler.OP_TYPES = {
    WITH_INPUT = "with_input",
    WITH_DATA = "with_data",
    FUNC = "func",
    AGENT = "agent",
    CYCLE = "cycle",
    PARALLEL = "parallel",
    STATE = "state",
    USE = "use",
    AS = "as",
    TO = "to",
    ERROR_TO = "error_to",
    WHEN = "when"
}

local FlowGraph = {}
local flow_graph_mt = { __index = FlowGraph }

function FlowGraph.new()
    return setmetatable({
        operations = table.create(16, 0),
        nodes = table.create(0, 16),
        node_order = table.create(16, 0),
        edges = table.create(0, 16),
        references = table.create(0, 8),
        input_data = nil,
        input_name = nil,
        input_routes = table.create(4, 0),
        static_data_sources = table.create(4, 0),
        last_node_id = nil,
        last_static_id = nil,
        last_node_name = nil,
        last_route_from_static = false,
        pending_routes = table.create(8, 0),
        has_explicit_routing = false,
        session_parent_id = nil,
        forced_success_nodes = table.create(0, 4),
        forced_failure_nodes = table.create(0, 4),
        auto_chained = table.create(0, 16)
    }, flow_graph_mt)
end

function FlowGraph:add_operation(op_type, config)
    table.insert(self.operations, {
        type = op_type,
        config = config or {}
    })
    return self, nil
end

function FlowGraph:create_node(node_type, config, metadata)
    local node_id = uuid.v7()

    self.nodes[node_id] = {
        node_id = node_id,
        node_type = node_type,
        config = config or {},
        metadata = metadata or {},
        status = consts.STATUS.PENDING
    }

    table.insert(self.node_order, node_id)

    self.edges[node_id] = {
        targets = table.create(4, 0),
        error_targets = table.create(2, 0)
    }

    self.last_node_id = node_id
    self.last_static_id = nil
    return node_id, nil
end

function FlowGraph:create_static_data(data)
    local static_id = uuid.v7()

    table.insert(self.static_data_sources, {
        static_id = static_id,
        data = data,
        routes = table.create(4, 0)
    })

    self.last_static_id = static_id
    self.last_node_id = nil
    return static_id, nil
end

function FlowGraph:create_template_nodes(template, parent_node_id)
    if not template or not template.operations then
        return table.create(0, 0)
    end

    local template_node_ids = table.create(#template.operations, 0)
    local last_template_node_id = nil

    for _, op in ipairs(template.operations) do
        if op.type == compiler.OP_TYPES.FUNC then
            local template_node_id = uuid.v7()

            local config = {
                func_id = op.config.func_id,
                args = op.config.args,
                inputs = op.config.inputs,
                context = op.config.context,
                input_transform = op.config.input_transform
            }

            if last_template_node_id then
                local prev_node = (self.nodes[last_template_node_id] :: any)
                if not prev_node.config.data_targets then
                    prev_node.config.data_targets = table.create(1, 0)
                end
                table.insert(prev_node.config.data_targets, {
                    data_type = consts.DATA_TYPE.NODE_INPUT,
                    node_id = template_node_id,
                    discriminator = "default",
                    metadata = {
                        source_node_id = last_template_node_id
                    }
                })
            end

            local metadata = op.config.metadata or {}

            self.nodes[template_node_id] = {
                node_id = template_node_id,
                node_type = "userspace.dataflow.node.func:node",
                config = config,
                metadata = metadata,
                status = consts.STATUS.TEMPLATE,
                parent_node_id = parent_node_id
            }

            self.edges[template_node_id] = {
                targets = table.create(2, 0),
                error_targets = table.create(1, 0)
            }

            table.insert(self.node_order, template_node_id)
            table.insert(template_node_ids, template_node_id)
            last_template_node_id = template_node_id
        elseif op.type == compiler.OP_TYPES.AGENT then
            local template_node_id = uuid.v7()

            local config = {
                agent = op.config.agent_id,
                model = op.config.model,
                arena = op.config.arena,
                inputs = op.config.inputs,
                show_tool_calls = op.config.show_tool_calls,
                input_transform = op.config.input_transform
            }

            if last_template_node_id then
                local prev_node = (self.nodes[last_template_node_id] :: any)
                if not prev_node.config.data_targets then
                    prev_node.config.data_targets = table.create(1, 0)
                end
                table.insert(prev_node.config.data_targets, {
                    data_type = consts.DATA_TYPE.NODE_INPUT,
                    node_id = template_node_id,
                    discriminator = "default",
                    metadata = {
                        source_node_id = last_template_node_id
                    }
                })
            end

            local metadata = op.config.metadata or {}

            self.nodes[template_node_id] = {
                node_id = template_node_id,
                node_type = "userspace.dataflow.node.agent:node",
                config = config,
                metadata = metadata,
                status = consts.STATUS.TEMPLATE,
                parent_node_id = parent_node_id
            }

            self.edges[template_node_id] = {
                targets = table.create(2, 0),
                error_targets = table.create(1, 0)
            }

            table.insert(self.node_order, template_node_id)
            table.insert(template_node_ids, template_node_id)
            last_template_node_id = template_node_id
        elseif op.type == compiler.OP_TYPES.CYCLE then
            local template_node_id = uuid.v7()

            local config = {
                func_id = op.config.func_id,
                args = op.config.args,
                continue_condition = op.config.continue_condition,
                max_iterations = op.config.max_iterations,
                initial_state = op.config.initial_state,
                inputs = op.config.inputs,
                context = op.config.context,
                input_transform = op.config.input_transform
            }

            if last_template_node_id then
                local prev_node = (self.nodes[last_template_node_id] :: any)
                if not prev_node.config.data_targets then
                    prev_node.config.data_targets = table.create(1, 0)
                end
                table.insert(prev_node.config.data_targets, {
                    data_type = consts.DATA_TYPE.NODE_INPUT,
                    node_id = template_node_id,
                    source_node_id = last_template_node_id,
                    discriminator = "default"
                })
            end

            local metadata = op.config.metadata or {}

            self.nodes[template_node_id] = {
                node_id = template_node_id,
                node_type = "userspace.dataflow.node.cycle:cycle",
                config = config,
                metadata = metadata,
                status = consts.STATUS.TEMPLATE,
                parent_node_id = parent_node_id
            }

            self.edges[template_node_id] = {
                targets = table.create(2, 0),
                error_targets = table.create(1, 0)
            }

            table.insert(self.node_order, template_node_id)
            table.insert(template_node_ids, template_node_id)

            if op.config.template then
                local cycle_template_nodes = self:create_template_nodes(op.config.template, template_node_id)
                for _, child_id in ipairs(cycle_template_nodes) do
                    table.insert(template_node_ids, child_id)
                end
            end

            last_template_node_id = template_node_id
        end
    end

    if last_template_node_id then
        local last_node = (self.nodes[last_template_node_id] :: any)
        if not last_node.config.data_targets then
            last_node.config.data_targets = table.create(1, 0)
        end
        table.insert(last_node.config.data_targets, {
            data_type = consts.DATA_TYPE.NODE_OUTPUT,
            discriminator = "result",
            metadata = {
                source_node_id = last_template_node_id
            }
        })

        if not last_node.config.error_targets then
            last_node.config.error_targets = table.create(1, 0)
        end
        table.insert(last_node.config.error_targets, {
            data_type = consts.DATA_TYPE.NODE_OUTPUT,
            discriminator = "error",
            metadata = {
                source_node_id = last_template_node_id
            }
        })
    end

    return template_node_ids
end

function FlowGraph:add_reference(name, node_id)
    if self.references[name] then
        return nil, "Duplicate node name: " .. name
    end
    self.references[name] = node_id
    self.last_node_name = name
    return true, nil
end

function FlowGraph:resolve_reference(name)
    local node_id = self.references[name]
    if not node_id then
        return nil, "Undefined node reference: " .. name
    end
    return node_id, nil
end

function FlowGraph:compute_auto_chain()
    for i = 1, #self.node_order - 1 do
        local current_node_id = self.node_order[i]
        local next_node_id = self.node_order[i + 1]
        local current_node = (self.nodes[current_node_id] :: any)
        local next_node = (self.nodes[next_node_id] :: any)

        if not current_node.parent_node_id and not next_node.parent_node_id then
            local current_edges = (self.edges[current_node_id] :: any)

            local has_any_targets = false
            for _, edge in ipairs(current_edges.targets) do
                if edge.target_node_id or edge.is_workflow_terminal then
                    has_any_targets = true
                    break
                end
            end
            for _, edge in ipairs(current_edges.error_targets) do
                if edge.target_node_id or edge.is_workflow_terminal then
                    has_any_targets = true
                    break
                end
            end

            if not has_any_targets then
                self.auto_chained[current_node_id] = next_node_id
                table.insert(current_edges.targets, {
                    target_node_id = next_node_id,
                    discriminator = "default",
                    is_auto_chain = true,
                    metadata = {
                        source_node_id = current_node_id
                    }
                })
            end
        end
    end
end

function FlowGraph:detect_cycles()
    local visited = table.create(0, 32)
    local rec_stack = table.create(0, 32)

    local function dfs(node_id, path)
        if rec_stack[node_id] then
            local cycle_start = nil
            for i, id in ipairs(path) do
                if id == node_id then
                    cycle_start = i
                    break
                end
            end

            if cycle_start then
                local cycle = table.create(#path - cycle_start + 2, 0)
                for i = cycle_start, #path do
                    table.insert(cycle, path[i])
                end
                table.insert(cycle, node_id)
                return true, "Cycle detected: " .. table.concat(cycle, " -> ")
            end
            return true, "Cycle detected at node: " .. node_id
        end

        if visited[node_id] then
            return false, nil
        end

        visited[node_id] = true
        rec_stack[node_id] = true
        table.insert(path, node_id)

        local edges = (self.edges[node_id] :: any)
        if edges then
            for _, edge in ipairs(edges.targets) do
                if edge.target_node_id then
                    local has_cycle, cycle_desc = dfs(edge.target_node_id, path)
                    if has_cycle then
                        return true, cycle_desc
                    end
                end
            end
            for _, edge in ipairs(edges.error_targets) do
                if edge.target_node_id then
                    local has_cycle, cycle_desc = dfs(edge.target_node_id, path)
                    if has_cycle then
                        return true, cycle_desc
                    end
                end
            end
        end

        rec_stack[node_id] = false
        table.remove(path)
        return false, nil
    end

    for node_id, _ in pairs(self.nodes) do
        if not visited[node_id] then
            local has_cycle, cycle_desc = dfs(node_id, table.create(16, 0))
            if has_cycle then
                return true, cycle_desc
            end
        end
    end

    return false, nil
end

function compiler.build_graph(operations, session_context)
    if not operations or #operations == 0 then
        return nil, "No operations provided"
    end

    local graph = FlowGraph.new() :: any

    if session_context and session_context.node_id then
        graph.session_parent_id = session_context.node_id
    end

    for _, op in ipairs(operations) do
        if op.type == compiler.OP_TYPES.WITH_INPUT then
            graph.input_data = op.config.data
        elseif op.type == compiler.OP_TYPES.WITH_DATA then
            local static_id, err = graph:create_static_data(op.config.data)
            if err then
                return nil, err
            end
        elseif op.type == compiler.OP_TYPES.FUNC then
            local config = {
                func_id = op.config.func_id,
                args = op.config.args,
                inputs = op.config.inputs,
                context = op.config.context,
                input_transform = op.config.input_transform
            }

            local node_id, err = graph:create_node("userspace.dataflow.node.func:node", config, op.config.metadata)
            if err then
                return nil, err
            end
        elseif op.type == compiler.OP_TYPES.AGENT then
            local config = {
                agent = op.config.agent_id,
                model = op.config.model,
                arena = op.config.arena,
                inputs = op.config.inputs,
                show_tool_calls = op.config.show_tool_calls,
                input_transform = op.config.input_transform
            }

            local node_id, err = graph:create_node("userspace.dataflow.node.agent:node", config, op.config.metadata)
            if err then
                return nil, err
            end
        elseif op.type == compiler.OP_TYPES.CYCLE then
            local config = {
                func_id = op.config.func_id,
                args = op.config.args,
                continue_condition = op.config.continue_condition,
                max_iterations = op.config.max_iterations,
                initial_state = op.config.initial_state,
                inputs = op.config.inputs,
                context = op.config.context,
                input_transform = op.config.input_transform
            }

            local node_id, err = graph:create_node("userspace.dataflow.node.cycle:cycle", config, op.config.metadata)
            if err then
                return nil, err
            end

            if op.config.template then
                graph:create_template_nodes(op.config.template, node_id)
            end
        elseif op.type == compiler.OP_TYPES.PARALLEL then
            local config = {
                source_array_key = op.config.source_array_key,
                iteration_input_key = op.config.iteration_input_key,
                batch_size = op.config.batch_size,
                on_error = op.config.on_error,
                filter = op.config.filter,
                unwrap = op.config.unwrap,
                passthrough_keys = op.config.passthrough_keys,
                inputs = op.config.inputs,
                input_transform = op.config.input_transform
            }

            local node_id, err = graph:create_node("userspace.dataflow.node.parallel:parallel", config,
                op.config.metadata)
            if err then
                return nil, err
            end

            if op.config.template then
                graph:create_template_nodes(op.config.template, node_id)
            end
        elseif op.type == compiler.OP_TYPES.STATE then
            local config = {
                inputs = op.config.inputs,
                input_transform = op.config.input_transform,
                output_mode = op.config.output_mode,
                ignored_keys = op.config.ignored_keys
            }

            local node_id, err = graph:create_node("userspace.dataflow.node.state:state", config, op.config.metadata)
            if err then
                return nil, err
            end
        elseif op.type == compiler.OP_TYPES.USE then
            if op.config.operations then
                for _, template_op in ipairs(op.config.operations) do
                    table.insert(graph.operations, template_op)
                end
            end
        elseif op.type == compiler.OP_TYPES.AS then
            if graph.last_static_id then
                graph:add_reference(op.config.name, graph.last_static_id)
            elseif graph.input_data and not graph.input_name and not graph.last_node_id then
                graph.input_name = op.config.name
                graph:add_reference(op.config.name, "INPUT")
            elseif graph.last_node_id then
                local success, err = graph:add_reference(op.config.name, graph.last_node_id)
                if err then
                    return nil, err
                end
            else
                return nil, "Cannot name node: no previous node, input, or static data to name: " .. op.config.name
            end
        elseif op.type == compiler.OP_TYPES.TO then
            if op.config.target == "@success" or op.config.target == "@fail" or op.config.target == "@end" then
                if not graph.last_node_id then
                    return nil, "Cannot route to terminal: no source node"
                end

                local is_success = op.config.target == "@success" or (op.config.target == "@end")

                if is_success then
                    graph.forced_success_nodes[graph.last_node_id] = true
                else
                    graph.forced_failure_nodes[graph.last_node_id] = true
                end

                graph.has_explicit_routing = true
                graph.last_route_from_static = false
                table.insert(graph.pending_routes, {
                    from_node_id = graph.last_node_id,
                    is_workflow_terminal = true,
                    is_success = is_success,
                    transform = op.config.transform,
                    condition = nil,
                    is_error = false
                })
            elseif graph.input_data and not graph.last_node_id and not graph.last_static_id then
                table.insert(graph.input_routes, {
                    target_name = op.config.target,
                    input_key = op.config.input_key or graph.input_name or "default",
                    transform = op.config.transform
                })
                graph.has_explicit_routing = true
                graph.last_route_from_static = false
            elseif graph.last_static_id then
                for _, static_source in ipairs(graph.static_data_sources) do
                    if (static_source :: any).static_id == graph.last_static_id then
                        table.insert((static_source :: any).routes, {
                            target_name = op.config.target,
                            input_key = op.config.input_key or graph.last_node_name or "default",
                            transform = op.config.transform
                        })
                        break
                    end
                end
                graph.has_explicit_routing = true
                graph.last_route_from_static = true
            elseif graph.last_node_id then
                graph.has_explicit_routing = true
                graph.last_route_from_static = false
                local discriminator = op.config.input_key
                if not discriminator and graph.last_node_name then
                    discriminator = graph.last_node_name
                end
                table.insert(graph.pending_routes, {
                    from_node_id = graph.last_node_id,
                    target_name = op.config.target,
                    input_key = discriminator,
                    transform = op.config.transform,
                    is_error = false,
                    condition = nil
                })
            else
                return nil, "Cannot add route: no source node, input, or static data"
            end
        elseif op.type == compiler.OP_TYPES.ERROR_TO then
            if op.config.target == "@success" or op.config.target == "@fail" or op.config.target == "@end" then
                if not graph.last_node_id then
                    return nil, "Cannot route to terminal: no source node"
                end

                local is_success = op.config.target == "@success"

                if is_success then
                    graph.forced_success_nodes[graph.last_node_id] = true
                else
                    graph.forced_failure_nodes[graph.last_node_id] = true
                end

                graph.has_explicit_routing = true
                graph.last_route_from_static = false
                table.insert(graph.pending_routes, {
                    from_node_id = graph.last_node_id,
                    is_workflow_terminal = true,
                    is_success = is_success,
                    transform = op.config.transform,
                    is_error = true,
                    condition = nil
                })
            else
                if not graph.last_node_id then
                    return nil, "Cannot add error route: no source node"
                end
                graph.has_explicit_routing = true
                graph.last_route_from_static = false
                local discriminator = op.config.input_key
                if not discriminator and graph.last_node_name then
                    discriminator = graph.last_node_name
                end
                table.insert(graph.pending_routes, {
                    from_node_id = graph.last_node_id,
                    target_name = op.config.target,
                    input_key = discriminator,
                    transform = op.config.transform,
                    is_error = true,
                    condition = nil
                })
            end
        elseif op.type == compiler.OP_TYPES.WHEN then
            if graph.last_route_from_static then
                return nil,
                    "Cannot use :when() with static data routes. Static data is constant and conditions would always evaluate the same way."
            end
            if #graph.pending_routes == 0 then
                return nil, "Cannot add condition: no preceding route from a node"
            end
            (graph.pending_routes[#graph.pending_routes] :: any).condition = op.config.condition
        end

        local success, err = graph:add_operation(op.type, op.config)
        if err then
            return nil, err
        end
    end

    for _, route in ipairs(graph.pending_routes) do
        local route_entry = (route :: any)
        if route_entry.is_workflow_terminal then
            local edges = (graph.edges[route_entry.from_node_id] :: any)
            local edge_list = route_entry.is_error and edges.error_targets or edges.targets
            table.insert(edge_list, {
                target_node_id = nil,
                is_workflow_terminal = true,
                is_success = route_entry.is_success,
                transform = route_entry.transform,
                condition = route_entry.condition
            })
        else
            local target_node_id, resolve_err = graph:resolve_reference(route_entry.target_name)
            if resolve_err then
                return nil, resolve_err
            end
            local edges = (graph.edges[route_entry.from_node_id] :: any)
            local edge_list = route_entry.is_error and edges.error_targets or edges.targets
            table.insert(edge_list, {
                target_node_id = target_node_id,
                transform = route_entry.transform,
                condition = route_entry.condition,
                input_key = route_entry.input_key
            })
        end
    end

    graph:compute_auto_chain()

    local has_cycles, cycle_desc = graph:detect_cycles()
    if has_cycles then
        return nil, "Flow contains cycles: " .. cycle_desc
    end

    return graph, nil
end

function compiler.find_root_nodes(graph)
    local nodes_with_incoming = table.create(0, 32)

    for _, edges in pairs((graph :: any).edges) do
        local edge_set = (edges :: any)
        for _, edge in ipairs(edge_set.targets) do
            if edge.target_node_id and not edge.is_auto_chain then
                nodes_with_incoming[edge.target_node_id] = true
            end
        end
        for _, edge in ipairs(edge_set.error_targets) do
            if edge.target_node_id then
                nodes_with_incoming[edge.target_node_id] = true
            end
        end
    end

    local roots = table.create(8, 0)
    for node_id, node_def in pairs((graph :: any).nodes) do
        if not nodes_with_incoming[node_id] and not (node_def :: any).parent_node_id then
            table.insert(roots, node_id)
        end
    end

    return roots, nil
end

function compiler.find_leaf_nodes(graph)
    local leaves = table.create(8, 0)

    for node_id, edges in pairs((graph :: any).edges) do
        local edge_set = (edges :: any)
        local has_node_targets = false
        for _, edge in ipairs(edge_set.targets) do
            if edge.target_node_id then
                has_node_targets = true
                break
            end
        end
        for _, edge in ipairs(edge_set.error_targets) do
            if edge.target_node_id then
                has_node_targets = true
                break
            end
        end
        if not has_node_targets then
            table.insert(leaves, node_id)
        end
    end

    return leaves, nil
end

function compiler.validate_graph(graph)
    local nodes_with_incoming = table.create(0, 32)

    for _, edges in pairs((graph :: any).edges) do
        local edge_set = (edges :: any)
        for _, edge in ipairs(edge_set.targets) do
            if edge.target_node_id then
                nodes_with_incoming[edge.target_node_id] = true
            end
        end
        for _, edge in ipairs(edge_set.error_targets) do
            if edge.target_node_id then
                nodes_with_incoming[edge.target_node_id] = true
            end
        end
    end

    local has_workflow_input = (graph :: any).input_data ~= nil
    local input_target_nodes = table.create(0, 8)

    if has_workflow_input then
        if #(graph :: any).input_routes > 0 then
            for _, route in ipairs((graph :: any).input_routes) do
                local route_entry = (route :: any)
                local target_id, err = (graph :: any):resolve_reference(route_entry.target_name)
                if not err and target_id then
                    input_target_nodes[target_id] = true
                end
            end
        else
            local root_nodes, _ = compiler.find_root_nodes(graph)
            if root_nodes then
                for _, node_id in ipairs(root_nodes) do
                    input_target_nodes[node_id] = true
                end
            end
        end
    end

    local static_target_nodes = table.create(0, 8)
    for _, static_source in ipairs((graph :: any).static_data_sources) do
        local src = (static_source :: any)
        for _, route in ipairs(src.routes) do
            local route_entry = (route :: any)
            local target_id, err = (graph :: any):resolve_reference(route_entry.target_name)
            if not err and target_id then
                static_target_nodes[target_id] = true
            end
        end
    end

    local dead_nodes = table.create(8, 0)
    for node_id, node_def in pairs((graph :: any).nodes) do
        local nd = (node_def :: any)
        if nd.status ~= consts.STATUS.TEMPLATE and not nd.parent_node_id then
            local has_incoming = nodes_with_incoming[node_id]
            local has_input = input_target_nodes[node_id]
            local has_static = static_target_nodes[node_id]

            if not has_incoming and not has_input and not has_static then
                local title = nd.metadata and nd.metadata.title or "unnamed"
                table.insert(dead_nodes, string.format("%s (%s)", title, node_id:sub(1, 12)))
            end
        end
    end

    if #dead_nodes > 0 then
        return false, string.format(
            "Dead nodes detected (no incoming routes): %s. All nodes must either receive data from another node, workflow input, or static data.",
            table.concat(dead_nodes, ", ")
        )
    end

    local nodes_with_default_inputs = table.create(0, 32)

    for source_node_id, edges in pairs((graph :: any).edges) do
        local edge_set = (edges :: any)
        for _, edge in ipairs(edge_set.targets) do
            if edge.target_node_id then
                local discriminator = edge.input_key or "default"
                if discriminator == "default" or discriminator == "" then
                    nodes_with_default_inputs[edge.target_node_id] = true
                end
            end
        end
    end

    if (graph :: any).input_data then
        if #(graph :: any).input_routes > 0 then
            for _, route in ipairs((graph :: any).input_routes) do
                local route_entry = (route :: any)
                local target_id, err = (graph :: any):resolve_reference(route_entry.target_name)
                if not err and target_id then
                    local discriminator = route_entry.input_key or "default"
                    if discriminator == "default" or discriminator == "" then
                        nodes_with_default_inputs[target_id] = true
                    end
                end
            end
        else
            local root_nodes, _ = compiler.find_root_nodes(graph)
            if root_nodes then
                for _, node_id in ipairs(root_nodes) do
                    nodes_with_default_inputs[node_id] = true
                end
            end
        end
    end

    for _, static_source in ipairs((graph :: any).static_data_sources) do
        local src = (static_source :: any)
        for _, route in ipairs(src.routes) do
            local route_entry = (route :: any)
            local target_id, err = (graph :: any):resolve_reference(route_entry.target_name)
            if not err and target_id then
                local discriminator = route_entry.input_key or "default"
                if discriminator == "default" or discriminator == "" then
                    nodes_with_default_inputs[target_id] = true
                end
            end
        end
    end

    local conflicts = table.create(8, 0)
    for node_id, node_def in pairs((graph :: any).nodes) do
        local nd = (node_def :: any)
        if nodes_with_default_inputs[node_id] then
            local has_args = nd.config.args ~= nil
            local has_string_transform = type(nd.config.input_transform) == "string"

            if has_args or has_string_transform then
                local title = nd.metadata and nd.metadata.title or "unnamed"
                local reason = ""
                if has_args then
                    reason = " (has args)"
                elseif has_string_transform then
                    reason = " (has string input_transform)"
                end
                table.insert(conflicts, string.format("%s (%s)%s", title, node_id:sub(1, 12), reason))
            end
        end
    end

    if #conflicts > 0 then
        return false, string.format(
            "Nodes with base arguments (args) cannot receive inputs with 'default' discriminator: %s. " ..
            "Use named discriminators with :to(target, 'input_key') or input_transform table form {key = 'expr'}.",
            table.concat(conflicts, ", ")
        )
    end

    local has_success_terminal = false
    local has_auto_output = false
    local leaf_nodes = table.create(8, 0)

    for node_id, edges in pairs((graph :: any).edges) do
        local edge_set = (edges :: any)
        local node_def = (graph :: any).nodes[node_id]
        if node_def and (node_def :: any).status ~= consts.STATUS.TEMPLATE then
            local nd = (node_def :: any)
            local has_node_targets = false
            for _, edge in ipairs(edge_set.targets) do
                if edge.target_node_id then
                    has_node_targets = true
                    break
                end
                if edge.is_workflow_terminal and edge.is_success then
                    has_success_terminal = true
                end
            end

            if not has_node_targets and not nd.parent_node_id then
                table.insert(leaf_nodes, {
                    node_id = node_id,
                    has_success_route = #edge_set.targets > 0,
                    has_error_route = #edge_set.error_targets > 0,
                    metadata = nd.metadata
                })
            end
        end
    end

    for _, leaf_info in ipairs(leaf_nodes) do
        local li = (leaf_info :: any)
        if not li.has_success_route and not li.has_error_route then
            has_auto_output = true
            break
        end
    end

    if not has_success_terminal and not has_auto_output then
        local problematic_nodes = table.create(#leaf_nodes, 0)
        for _, leaf_info in ipairs(leaf_nodes) do
            local li = (leaf_info :: any)
            if li.has_error_route and not li.has_success_route then
                local title = li.metadata and li.metadata.title or "unnamed"
                table.insert(problematic_nodes, string.format("%s (%s)", title, li.node_id:sub(1, 12)))
            end
        end

        if #problematic_nodes > 0 then
            return false, string.format(
                "Workflow has no success termination path. Node(s) with :error_to() but no :to() route: %s. " ..
                "Add :to(\"@success\") to at least one node to complete the workflow on success.",
                table.concat(problematic_nodes, ", ")
            )
        else
            return false, "Workflow has no completion path. Add :to(\"@success\") to at least one leaf node."
        end
    end

    return true, nil
end

function compiler.compile_to_commands(graph, session_context)
    if not graph then
        return nil, "Graph is required"
    end

    local commands = table.create(#(graph :: any).node_order * 2, 0)
    local input_data_id = nil
    local is_nested = session_context and session_context.dataflow_id

    -- Step 1: Create workflow input data object (only for non-nested workflows)
    if (graph :: any).input_data and not is_nested then
        input_data_id = uuid.v7()
        table.insert(commands, {
            type = consts.COMMAND_TYPES.CREATE_DATA,
            payload = {
                data_id = input_data_id,
                data_type = consts.DATA_TYPE.WORKFLOW_INPUT,
                content = (graph :: any).input_data,
                content_type = type((graph :: any).input_data) == "table" and consts.CONTENT_TYPE.JSON or consts.CONTENT_TYPE.TEXT
            }
        })
    end

    -- Step 2: Create all nodes first
    local leaf_nodes, leaf_err = compiler.find_leaf_nodes(graph)
    if leaf_err then
        return nil, leaf_err
    end

    for _, node_id in ipairs((graph :: any).node_order) do
        local node_def = ((graph :: any).nodes[node_id] :: any)
        local config = {}

        for k, v in pairs(node_def.config) do
            config[k] = v
        end

        local edges = ((graph :: any).edges[node_id] :: any)
        local has_explicit_edges = false

        for _, edge in ipairs(edges.targets) do
            if edge.target_node_id or edge.is_workflow_terminal then
                has_explicit_edges = true
                break
            end
        end
        for _, edge in ipairs(edges.error_targets) do
            if edge.target_node_id or edge.is_workflow_terminal then
                has_explicit_edges = true
                break
            end
        end

        if has_explicit_edges then
            config.data_targets = table.create(#edges.targets, 0)
            config.error_targets = table.create(#edges.error_targets, 0)

            for _, edge in ipairs(edges.targets) do
                if edge.is_workflow_terminal then
                    local has_parent = node_def.parent_node_id or (graph :: any).session_parent_id
                    local output_type = has_parent and consts.DATA_TYPE.NODE_OUTPUT or consts.DATA_TYPE.WORKFLOW_OUTPUT

                    local target = {
                        data_type = output_type,
                        discriminator = edge.is_success and "result" or "error",
                        condition = edge.condition,
                        transform = edge.transform,
                        metadata = {
                            source_node_id = node_id
                        }
                    }

                    if output_type == consts.DATA_TYPE.NODE_OUTPUT then
                        (target :: any).node_id = node_id
                    end

                    table.insert(config.data_targets, target)
                else
                    table.insert(config.data_targets, {
                        data_type = consts.DATA_TYPE.NODE_INPUT,
                        node_id = edge.target_node_id,
                        discriminator = edge.input_key or "default",
                        condition = edge.condition,
                        transform = edge.transform,
                        metadata = {
                            source_node_id = node_id
                        }
                    })
                end
            end

            for _, edge in ipairs(edges.error_targets) do
                if edge.is_workflow_terminal then
                    local has_parent = node_def.parent_node_id or (graph :: any).session_parent_id
                    local output_type = has_parent and consts.DATA_TYPE.NODE_OUTPUT or consts.DATA_TYPE.WORKFLOW_OUTPUT

                    local target = {
                        data_type = output_type,
                        discriminator = edge.is_success and "result" or "error",
                        condition = edge.condition,
                        transform = edge.transform,
                        metadata = {
                            source_node_id = node_id
                        }
                    }

                    if output_type == consts.DATA_TYPE.NODE_OUTPUT then
                        (target :: any).node_id = node_id
                    end

                    table.insert(config.error_targets, target)
                else
                    table.insert(config.error_targets, {
                        data_type = consts.DATA_TYPE.NODE_INPUT,
                        node_id = edge.target_node_id,
                        discriminator = edge.input_key or "default",
                        condition = edge.condition,
                        transform = edge.transform,
                        metadata = {
                            source_node_id = node_id
                        }
                    })
                end
            end
        else
            local is_leaf = false
            local is_template = node_def.status == consts.STATUS.TEMPLATE

            for _, leaf_id in ipairs(leaf_nodes) do
                if leaf_id == node_id then
                    is_leaf = true
                    break
                end
            end

            if is_leaf and not is_template then
                local has_parent = node_def.parent_node_id or (graph :: any).session_parent_id
                local output_data_type = has_parent and consts.DATA_TYPE.NODE_OUTPUT or consts.DATA_TYPE.WORKFLOW_OUTPUT

                config.data_targets = table.create(1, 0)

                local target = {
                    data_type = output_data_type,
                    discriminator = "result",
                    content_type = consts.CONTENT_TYPE.JSON,
                    metadata = {
                        source_node_id = node_id
                    }
                }

                if output_data_type == consts.DATA_TYPE.NODE_OUTPUT then
                    (target :: any).node_id = node_id
                end

                table.insert(config.data_targets, target)
            end
        end

        local node_payload = {
            node_id = node_id,
            node_type = node_def.node_type,
            status = node_def.status,
            config = config,
            metadata = node_def.metadata
        } :: any

        if node_def.parent_node_id then
            node_payload.parent_node_id = node_def.parent_node_id
        elseif (graph :: any).session_parent_id then
            node_payload.parent_node_id = (graph :: any).session_parent_id
        end

        table.insert(commands, {
            type = consts.COMMAND_TYPES.CREATE_NODE,
            payload = node_payload
        })
    end

    -- Step 3: Create static data sources (nodes now exist)
    local static_data_ids = table.create(0, #(graph :: any).static_data_sources)

    for _, static_source in ipairs((graph :: any).static_data_sources) do
        local src = (static_source :: any)
        if #src.routes > 0 then
            local first_route = (src.routes[1] :: any)
            local target_node_id, err = (graph :: any):resolve_reference(first_route.target_name)
            if err then
                return nil, err
            end

            local content = src.data
            if first_route.transform then
                local transform_env = {
                    output = src.data
                }
                local transformed, eval_err = expr.eval(first_route.transform :: string, transform_env)
                if eval_err then
                    return nil, "Static data route transform failed: " .. (eval_err :: string)
                end
                content = transformed
            end

            local data_id = uuid.v7()
            static_data_ids[src.static_id] = data_id

            table.insert(commands, {
                type = consts.COMMAND_TYPES.CREATE_DATA,
                payload = {
                    data_id = data_id,
                    data_type = consts.DATA_TYPE.NODE_INPUT,
                    node_id = target_node_id,
                    discriminator = first_route.input_key,
                    content = content,
                    content_type = type(content) == "table" and consts.CONTENT_TYPE.JSON or consts.CONTENT_TYPE.TEXT
                }
            })

            for i = 2, #src.routes do
                local route = (src.routes[i] :: any)
                local route_target_id, route_err = (graph :: any):resolve_reference(route.target_name)
                if route_err then
                    return nil, route_err
                end

                table.insert(commands, {
                    type = consts.COMMAND_TYPES.CREATE_DATA,
                    payload = {
                        data_id = uuid.v7(),
                        data_type = consts.DATA_TYPE.NODE_INPUT,
                        node_id = route_target_id,
                        discriminator = route.input_key,
                        key = data_id,
                        content = "",
                        content_type = consts.CONTENT_TYPE.REFERENCE
                    }
                })
            end
        end
    end

    -- Step 4: Create nested workflow input routing (nodes now exist)
    if (graph :: any).input_data and is_nested then
        if #(graph :: any).input_routes > 0 then
            for _, route in ipairs((graph :: any).input_routes) do
                local route_entry = (route :: any)
                local target_node_id, err = (graph :: any):resolve_reference(route_entry.target_name)
                if err then
                    return nil, err
                end

                local content = (graph :: any).input_data
                if route_entry.transform then
                    local transform_env = {
                        input = (graph :: any).input_data,
                        output = (graph :: any).input_data
                    }
                    local transformed, eval_err = expr.eval(route_entry.transform :: string, transform_env)
                    if eval_err then
                        return nil, "Input route transform failed: " .. (eval_err :: string)
                    end
                    content = transformed
                end

                table.insert(commands, {
                    type = consts.COMMAND_TYPES.CREATE_DATA,
                    payload = {
                        data_id = uuid.v7(),
                        data_type = consts.DATA_TYPE.NODE_INPUT,
                        node_id = target_node_id,
                        discriminator = route_entry.input_key,
                        content = content,
                        content_type = type(content) == "table" and consts.CONTENT_TYPE.JSON or consts.CONTENT_TYPE.TEXT
                    }
                })
            end
        else
            local root_nodes, roots_err = compiler.find_root_nodes(graph)
            if roots_err then
                return nil, roots_err
            end

            for _, node_id in ipairs(root_nodes) do
                table.insert(commands, {
                    type = consts.COMMAND_TYPES.CREATE_DATA,
                    payload = {
                        data_id = uuid.v7(),
                        data_type = consts.DATA_TYPE.NODE_INPUT,
                        node_id = node_id,
                        discriminator = "default",
                        content = (graph :: any).input_data,
                        content_type = type((graph :: any).input_data) == "table" and consts.CONTENT_TYPE.JSON or
                            consts.CONTENT_TYPE.TEXT
                    }
                })
            end
        end
    end

    -- Step 5: Create workflow input data references (non-nested, nodes now exist)
    if input_data_id and not is_nested then
        local root_nodes, roots_err = compiler.find_root_nodes(graph)
        if roots_err then
            return nil, roots_err
        end

        if #(graph :: any).input_routes > 0 then
            for _, route in ipairs((graph :: any).input_routes) do
                local route_entry = (route :: any)
                local target_node_id, err = (graph :: any):resolve_reference(route_entry.target_name)
                if err then
                    return nil, err
                end

                table.insert(commands, {
                    type = consts.COMMAND_TYPES.CREATE_DATA,
                    payload = {
                        data_id = uuid.v7(),
                        data_type = consts.DATA_TYPE.NODE_INPUT,
                        node_id = target_node_id,
                        key = input_data_id,
                        discriminator = route_entry.input_key,
                        content = "",
                        content_type = "dataflow/reference"
                    }
                })
            end
        else
            for _, node_id in ipairs(root_nodes) do
                table.insert(commands, {
                    type = consts.COMMAND_TYPES.CREATE_DATA,
                    payload = {
                        data_id = uuid.v7(),
                        data_type = consts.DATA_TYPE.NODE_INPUT,
                        node_id = node_id,
                        key = input_data_id,
                        discriminator = "default",
                        content = "",
                        content_type = "dataflow/reference"
                    }
                })
            end
        end
    end

    return commands, nil
end

function compiler.compile(operations, session_context)
    if not operations or #operations == 0 then
        return nil, "No operations to compile"
    end

    local graph, graph_err = compiler.build_graph(operations, session_context)
    if graph_err then
        return nil, graph_err
    end

    local valid, validation_err = compiler.validate_graph(graph)
    if not valid then
        return nil, validation_err
    end

    local commands, commands_err = compiler.compile_to_commands(graph, session_context)
    if commands_err then
        return nil, commands_err
    end

    return {
        commands = commands,
        graph = graph
    }, nil
end

return compiler
