local json = require("json")
local uuid = require("uuid")
local expr = require("expr")
local consts = require("consts")

local default_deps = {
    commit = require("commit"),
    data_reader = require("data_reader"),
    process = process
}

local node = {}
local methods = {}
local mt = { __index = methods }

local function merge_metadata(existing, new_fields)
    local existing_count = 0
    local new_count = 0

    if type(existing) == "table" then
        for _ in pairs(existing) do
            existing_count = existing_count + 1
        end
    end

    if type(new_fields) == "table" then
        for _ in pairs(new_fields) do
            new_count = new_count + 1
        end
    end

    local result = table.create(0, existing_count + new_count)

    if type(existing) == "table" then
        for k, v in pairs(existing) do
            result[k] = v
        end
    end
    if type(new_fields) == "table" then
        for k, v in pairs(new_fields) do
            result[k] = v
        end
    end
    return result
end

local function create_transform_env(raw_inputs)
    local input_count = 0
    for _ in pairs(raw_inputs) do
        input_count = input_count + 1
    end

    local inputs_by_key = table.create(0, input_count)
    local default_content = nil

    for key, input_data in pairs(raw_inputs) do
        inputs_by_key[key] = input_data.content

        if key == "default" or key == "" then
            default_content = input_data.content
        end
    end

    return {
        input = raw_inputs,
        inputs = inputs_by_key,
        default = default_content
    }
end

local function resolve_dataflow_references(self, value)
    if type(value) ~= "table" then
        return value
    end

    if value._dataflow_ref then
        local reader = (self._deps.data_reader.with_dataflow(self.dataflow_id) :: any)
            :with_data(value._dataflow_ref)
            :fetch_options({ replace_references = true })

        local data = reader:one()
        if data and data.content then
            if data.content_type == consts.CONTENT_TYPE.JSON and type(data.content) == "string" then
                local parsed, _ = json.decode(data.content)
                return parsed or data.content
            end
            return data.content
        end
        return value
    end

    if #value > 0 and value[1] and value[1]._dataflow_ref then
        local resolved = {}
        for i, item in ipairs(value) do
            resolved[i] = resolve_dataflow_references(self, item)
        end
        return resolved
    end

    return value
end

function node.new(args, deps)
    if not args then
        return nil, "Node args required"
    end
    if not args.node_id or not args.dataflow_id then
        return nil, "Node args must contain node_id and dataflow_id"
    end

    deps = deps or default_deps

    local yield_reply_topic = consts.MESSAGE_TOPIC.YIELD_REPLY_PREFIX .. args.node_id
    local yield_channel = (deps.process :: any).listen(yield_reply_topic)

    local instance = {
        node_id = args.node_id,
        dataflow_id = args.dataflow_id,
        node = args.node or {},
        path = args.path or table.create(1, 0),

        _config = (args.node and args.node.config) or {},
        data_targets = (args.node and args.node.config and args.node.config.data_targets) or table.create(0, 0),
        error_targets = (args.node and args.node.config and args.node.config.error_targets) or table.create(0, 0),

        _metadata = (args.node and args.node.metadata) or {},
        _queued_commands = table.create(10, 0),
        _created_data_ids = table.create(5, 0),
        _cached_inputs = nil,

        _yield_channel = yield_channel,
        _yield_reply_topic = yield_reply_topic,
        _last_yield_id = nil,

        _deps = deps
    }

    if not instance.path[1] or instance.path[1] ~= args.node_id then
        table.insert(instance.path, args.node_id)
    end

    return setmetatable(instance, mt) :: any, nil
end

function methods:config()
    return self._config
end

function methods:_transform_inputs_with_expr(raw_inputs, transform_config)
    local env = create_transform_env(raw_inputs)

    if type(transform_config) == "string" then
        local content, err = expr.eval(transform_config, env)
        if err then
            return nil, "Input transformation failed: " .. tostring(err)
        end
        return {
            ["default"] = {
                content = content,
                metadata = {},
                key = "default",
                discriminator = nil
            }
        }, nil
    end

    if type(transform_config) ~= "table" then
        return nil, "Node [" .. self.node_id .. "] input_transform must be string or table"
    end

    local field_count = 0
    for _ in pairs(transform_config :: {[string]: string}) do
        field_count = field_count + 1
    end

    local result = table.create(0, field_count)
    for field_name, expression in pairs(transform_config :: {[string]: string}) do
        if type(expression) ~= "string" then
            return nil, "Transform failed for " .. field_name .. ": expression must be a string"
        end

        local content, err = expr.eval(expression, env)
        if err then
            return nil, "Transform failed for " .. field_name .. ": " .. tostring(err)
        end
        result[field_name] = {
            content = content,
            metadata = {},
            key = field_name,
            discriminator = nil
        }
    end
    return result, nil
end

function methods:_load_raw_inputs()
    local input_data = (self._deps.data_reader.with_dataflow(self.dataflow_id) :: any)
        :with_nodes(self.node_id)
        :with_data_types(consts.DATA_TYPE.NODE_INPUT)
        :fetch_options({ replace_references = true })
        :all()

    local inputs_map = table.create(0, #input_data)

    for _, input in ipairs(input_data) do
        local parsed_content = input.content

        if input.content_type == consts.CONTENT_TYPE.JSON and type(input.content) == "string" then
            local parsed, err = json.decode(input.content)
            if not err then
                parsed_content = parsed
            end
        end

        local map_key = input.discriminator or input.key or "default"
        inputs_map[map_key] = {
            content = parsed_content,
            metadata = input.metadata or {},
            key = input.key,
            discriminator = input.discriminator,
            data_id = input.data_id,
            content_type = input.content_type
        }
    end

    return inputs_map
end

function methods:inputs()
    if self._cached_inputs then
        return self._cached_inputs
    end

    local raw_inputs = self:_load_raw_inputs()

    local transform_config = self._config.input_transform
    if transform_config then
        local transformed, err = self:_transform_inputs_with_expr(raw_inputs, transform_config)
        if err then
            error(err)
        end
        self._cached_inputs = transformed
        return transformed, nil
    end

    self._cached_inputs = raw_inputs
    return raw_inputs, nil
end

function methods:input(key)
    if not key then
        error("Input key is required")
    end

    local inputs_map, err = self:inputs()
    if err then
        error(err)
    end
    return inputs_map[key], nil
end

function methods:with_child_nodes(definitions)
    if not definitions or type(definitions) ~= "table" then
        return nil, "Child definitions required"
    end

    local child_ids = table.create(#definitions, 0)

    for i, definition in ipairs(definitions) do
        if type(definition) ~= "table" then
            return nil, "Invalid child definition at index " .. i
        end

        local node_type = definition.node_type
        if type(node_type) ~= "string" or node_type == "" then
            return nil, "Child definition at index " .. i .. " missing node_type"
        end

        local child_id = definition.node_id or uuid.v7()
        child_ids[i] = child_id

        table.insert(self._queued_commands, {
            type = consts.COMMAND_TYPES.CREATE_NODE,
            payload = {
                node_id = child_id,
                node_type = node_type,
                parent_node_id = self.node_id,
                status = definition.status or consts.STATUS.PENDING,
                config = definition.config,
                metadata = definition.metadata
            }
        })
    end

    return child_ids, nil
end

function methods:data(data_type, content, options)
    if not data_type or data_type == "" then
        return nil, "Node [" .. self.node_id .. "] data type is required"
    end

    if content == nil then
        return nil, "Node [" .. self.node_id .. "] content is required"
    end

    options = options or {}

    local content_type = options.content_type
    if not content_type then
        if type(content) == "table" then
            content_type = consts.CONTENT_TYPE.JSON
        else
            content_type = consts.CONTENT_TYPE.TEXT
        end
    end

    local data_id = options.data_id or uuid.v7()
    table.insert(self._created_data_ids, data_id)

    local command = {
        type = consts.COMMAND_TYPES.CREATE_DATA,
        payload = {
            data_id = data_id,
            data_type = data_type,
            key = options.key,
            content = content,
            content_type = content_type,
            discriminator = options.discriminator,
            node_id = options.node_id,
            metadata = options.metadata
        }
    }

    table.insert(self._queued_commands, command)
    return self, nil
end

function methods:update_metadata(updates)
    if not updates or type(updates) ~= "table" then
        return self, nil
    end

    self._metadata = merge_metadata(self._metadata, updates)

    local command = {
        type = consts.COMMAND_TYPES.UPDATE_NODE,
        payload = {
            node_id = self.node_id,
            metadata = self._metadata
        }
    }

    table.insert(self._queued_commands, command)
    return self, nil
end

function methods:update_config(updates)
    if not updates or type(updates) ~= "table" then
        return self, nil
    end

    self._config = merge_metadata(self._config, updates)

    local command = {
        type = consts.COMMAND_TYPES.UPDATE_NODE,
        payload = {
            node_id = self.node_id,
            config = self._config
        }
    }

    table.insert(self._queued_commands, command)
    return self, nil
end

function methods:submit()
    if #self._queued_commands == 0 then
        return true, nil
    end

    local op_id = uuid.v7()
    local success, err = self._deps.commit.submit(self.dataflow_id, op_id, self._queued_commands)

    if success then
        self._queued_commands = table.create(10, 0)
        return true, nil
    else
        return false, err or "unknown"
    end
end

function methods.yield(self: table, options)
    options = options or {}

    local yield_id = uuid.v7()
    local op_id = uuid.v7()

    local yield_command: table = {
        type = consts.COMMAND_TYPES.CREATE_DATA,
        payload = {
            data_id = uuid.v7(),
            data_type = consts.DATA_TYPE.NODE_YIELD,
            content = {
                node_id = self.node_id,
                yield_id = yield_id,
                reply_to = self._yield_reply_topic,
                yield_context = {
                    run_nodes = options.run_nodes or table.create(0, 0)
                }
            },
            content_type = consts.CONTENT_TYPE.JSON,
            key = yield_id,
            node_id = self.node_id
        }
    }
    table.insert(self._queued_commands, yield_command)

    local submitted, err = self._deps.commit.submit(self.dataflow_id, op_id, self._queued_commands)
    if not submitted then
        return nil, "Failed to submit yield: " .. (err or "unknown")
    end
    self._queued_commands = table.create(10, 0)

    local yield_signal = {
        request_context = {
            yield_id = yield_id,
            node_id = self.node_id,
            reply_to = self._yield_reply_topic
        },
        yield_context = {
            run_nodes = options.run_nodes or table.create(0, 0)
        }
    }

    local success = (self._deps.process :: any).send(
        "dataflow." .. self.dataflow_id,
        consts.MESSAGE_TOPIC.YIELD_REQUEST,
        yield_signal
    )

    if not success then
        return nil, "Failed to send yield signal"
    end

    local received, ok = self._yield_channel:receive()
    if not ok then
        return nil, "Yield channel closed"
    end

    self._last_yield_id = yield_id

    if received and received.response_data then
        return received.response_data.run_node_results or table.create(0, 0), nil
    end

    return table.create(0, 0), nil
end

function methods:query()
    return self._deps.data_reader.with_dataflow(self.dataflow_id)
end

function methods:_route_outputs(content)
    local routed_data_ids = table.create(#self.data_targets, 0)
    local data_id_count = 0

    local resolved_content = resolve_dataflow_references(self, content)

    local ok, inputs_or_err = pcall(function()
        local values = self:inputs()
        return values
    end)
    if not ok then
        return nil, "Node [" .. self.node_id .. "] failed to load inputs for output routing: " .. tostring(inputs_or_err)
    end

    local inputs_map = inputs_or_err
    local env = {
        output = resolved_content,
        input = inputs_map or {},
        inputs = inputs_map or {},
        node = self
    }

    for target_idx, target in ipairs(self.data_targets) do
        local target_desc = "target[" .. target_idx .. "]"
        if target.discriminator then
            target_desc = target_desc .. " (discriminator=" .. target.discriminator .. ")"
        end
        if target.node_id then
            target_desc = target_desc .. " -> node[" .. target.node_id .. "]"
        end

        if target.condition then
            local should_create, condition_err = expr.eval(target.condition :: string, env)
            if condition_err then
                return nil, "Output condition evaluation failed for " .. target_desc .. ": " .. tostring(condition_err)
            end
            if not should_create then
                goto continue
            end
        end

        local output_content = resolved_content
        local has_transform = target.transform ~= nil

        if has_transform then
            local transformed, transform_err = expr.eval(target.transform :: string, env)
            if transform_err then
                return nil, "Output transform failed for " .. target_desc .. ": " .. tostring(transform_err)
            end
            output_content = transformed
        end

        if output_content == nil then
            return nil, "Node [" .. self.node_id .. "] output content is nil for " .. target_desc ..
                       " (transform: " .. tostring(target.transform or "none") .. ")"
        end

        local data_id = uuid.v7()
        data_id_count = data_id_count + 1
        routed_data_ids[data_id_count] = data_id

        local _, data_err = self:data(target.data_type, output_content, {
            data_id = data_id,
            key = target.key,
            discriminator = target.discriminator,
            node_id = target.node_id or self.node_id,
            content_type = target.content_type,
            metadata = target.metadata
        })
        if data_err then
            return nil, "Node [" .. self.node_id .. "] failed to create data for " .. target_desc .. ": " .. (data_err :: string)
        end

        ::continue::
    end

    return routed_data_ids, nil
end

function methods:_route_errors(error_content)
    local routed_data_ids = table.create(#self.error_targets, 0)
    local data_id_count = 0

    local env = {
        error = error_content,
        node = self
    }

    for target_idx, target in ipairs(self.error_targets) do
        local target_desc = "error_target[" .. target_idx .. "]"
        if target.discriminator then
            target_desc = target_desc .. " (discriminator=" .. target.discriminator .. ")"
        end
        if target.node_id then
            target_desc = target_desc .. " -> node[" .. target.node_id .. "]"
        end

        if target.condition then
            local should_create, condition_err = expr.eval(target.condition :: string, env)
            if condition_err then
                goto continue
            end
            if not should_create then
                goto continue
            end
        end

        local error_output = error_content
        if target.transform then
            local transformed, transform_err = expr.eval(target.transform :: string, env)
            if not transform_err then
                error_output = transformed
            end
        end

        local data_id = uuid.v7()
        data_id_count = data_id_count + 1
        routed_data_ids[data_id_count] = data_id

        self:data(target.data_type, error_output, {
            data_id = data_id,
            key = target.key,
            discriminator = target.discriminator,
            node_id = target.node_id,
            content_type = target.content_type,
            metadata = target.metadata
        })

        ::continue::
    end

    return routed_data_ids, nil
end

function methods:_submit_final()
    if #self._queued_commands == 0 then
        return true, nil
    end

    local result, err = self._deps.commit.submit(
        self.dataflow_id,
        uuid.v7(),
        self._queued_commands
    )

    self._queued_commands = table.create(10, 0)
    return result ~= nil, err
end

function methods:complete(output_content, message, extra_metadata)
    if extra_metadata then
        local _, meta_err = self:update_metadata(extra_metadata)
        if meta_err then
            return {
                success = false,
                message = "Node [" .. self.node_id .. "] failed to update metadata: " .. (meta_err :: string),
                error = meta_err,
                data_ids = table.create(0, 0)
            }
        end
    end

    if message then
        local _, msg_err = self:update_metadata({ status_message = message })
        if msg_err then
            return {
                success = false,
                message = "Node [" .. self.node_id .. "] failed to set status message: " .. (msg_err :: string),
                error = msg_err,
                data_ids = table.create(0, 0)
            }
        end
    end

    local data_ids = table.create(#self.data_targets, 0)
    if output_content ~= nil then
        local routed_ids, route_err = self:_route_outputs(output_content)
        if route_err then
            error(route_err)
        end
        data_ids = routed_ids
    end

    local success, err = self:_submit_final()
    if not success then
        return {
            success = false,
            message = "Node [" .. self.node_id .. "] failed to submit final commands: " .. (err or "unknown"),
            error = err,
            data_ids = table.create(0, 0)
        }
    end

    return {
        success = true,
        message = message or "Node execution completed successfully",
        data_ids = data_ids
    }
end

function methods:fail(error_details, message, extra_metadata)
    local error_msg = error_details or "Unknown error"
    local status_msg = message or error_msg

    local error_metadata = {
        status_message = status_msg,
        error = error_msg
    }

    if extra_metadata then
        error_metadata = merge_metadata(error_metadata, extra_metadata)
    end

    self:update_metadata(error_metadata)

    local data_ids = table.create(#self.error_targets, 0)
    if error_details ~= nil then
        local routed_ids, route_err = self:_route_errors(error_details)
        if not route_err then
            data_ids = routed_ids
        end
    end

    local success, err = self:_submit_final()
    if not success then
        return {
            success = false,
            message = "Node [" .. self.node_id .. "] failed to submit final commands: " .. (err or "unknown"),
            error = err,
            data_ids = table.create(0, 0)
        }
    end

    return {
        success = false,
        message = status_msg,
        error = error_msg,
        data_ids = data_ids
    }
end

function methods:command(cmd)
    if not cmd or not cmd.type then
        return nil, "Command must have a type"
    end

    table.insert(self._queued_commands, cmd)
    return self, nil
end

function methods:created_data_ids()
    return self._created_data_ids
end

return node
