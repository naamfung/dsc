package typ

// ExtendRecordWithField returns a record type extended with a field.
// If the base type is nil, any, unknown, or nil, creates a new record with just the field.
// If the base type is already a record, adds or updates the field.
func ExtendRecordWithField(base Type, field string, fieldType Type) Type {
	if field == "" || fieldType == nil {
		return base
	}

	unwrapped := base
	for a, ok := unwrapped.(*Alias); ok; a, ok = unwrapped.(*Alias) {
		unwrapped = a.Target
	}
	if unwrapped == nil || unwrapped.Kind() == Any.Kind() || unwrapped.Kind() == Unknown.Kind() || unwrapped.Kind() == Nil.Kind() {
		return NewRecord().SetOpen(true).Field(field, fieldType).Build()
	}

	rec, ok := unwrapped.(*Record)
	if !ok {
		return base
	}

	builder := NewRecord()
	if rec.Open {
		builder.SetOpen(true)
	}
	added := false
	for _, f := range rec.Fields {
		if f.Name == field {
			builder.Field(f.Name, fieldType)
			added = true
			continue
		}
		if f.Optional {
			if f.Readonly {
				builder.OptReadonlyField(f.Name, f.Type)
			} else {
				builder.OptField(f.Name, f.Type)
			}
			continue
		}
		if f.Readonly {
			builder.ReadonlyField(f.Name, f.Type)
		} else {
			builder.Field(f.Name, f.Type)
		}
	}
	if !added {
		builder.Field(field, fieldType)
	}
	if rec.Metatable != nil {
		builder.Metatable(rec.Metatable)
	}
	if rec.HasMapComponent() {
		builder.MapComponent(rec.MapKey, rec.MapValue)
	}
	return builder.Build()
}
