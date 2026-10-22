package config

func clonePersona(p PersonaConfig) PersonaConfig {
	if p.IncludeRuntimeContext != nil {
		value := *p.IncludeRuntimeContext
		p.IncludeRuntimeContext = &value
	}
	return p
}
