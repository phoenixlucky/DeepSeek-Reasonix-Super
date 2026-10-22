package control

import (
	"errors"
	"fmt"
	"strings"

	"reasonix/internal/contract/config"
)

// ErrObservabilityUnwritable is a save the user file refused for a reason that
// is not about the value.
var ErrObservabilityUnwritable = errors.New("the trace export settings could not be saved")

// ObservabilitySettings is the user file's [observability.otlp] table. The export
// is built when a session opens, so a save reaches new sessions only.
type ObservabilitySettings struct {
	Enabled     bool   `json:"enabled"`
	Endpoint    string `json:"endpoint"`
	HeaderName  string `json:"headerName"`
	HeaderEnv   string `json:"headerEnv"`
	ServiceName string `json:"serviceName"`
	// HeaderEnvSet says the named variable has a value now; the value is never read back.
	HeaderEnvSet bool   `json:"headerEnvSet"`
	Path         string `json:"path"`
	Applies      string `json:"applies"`
}

// ObservabilityPatch is a partial update: a nil field is left as it is.
type ObservabilityPatch struct {
	Enabled     *bool
	Endpoint    *string
	HeaderName  *string
	HeaderEnv   *string
	ServiceName *string
}

func (c *Controller) ObservabilitySettings() ObservabilitySettings {
	return observabilityFrom(c.WorkspaceRoot(), config.UserOTLP())
}

func observabilityFrom(root string, o config.OTLPConfig) ObservabilitySettings {
	return ObservabilitySettings{Enabled: o.Enabled, Endpoint: o.Endpoint, HeaderName: o.HeaderName, HeaderEnv: o.HeaderEnv,
		ServiceName: o.ServiceName, HeaderEnvSet: o.HeaderEnv != "" && config.ResolveCredentialForRoot(root, o.HeaderEnv).Set, Path: config.UserConfigPath(), Applies: "next_session"}
}

// SaveObservabilitySettings writes the patch to the user file. A value the
// export could not use is refused with the config's own error, so a frontend
// maps it by identity.
func (c *Controller) SaveObservabilitySettings(patch ObservabilityPatch) (ObservabilitySettings, error) {
	got, err := config.UpdateUserOTLP(func(o *config.OTLPConfig) error {
		var edit config.Config
		edit.Observability.OTLP = *o
		if patch.Endpoint != nil {
			if err := edit.SetOTLPEndpoint(*patch.Endpoint); err != nil {
				return err
			}
		}
		if patch.HeaderName != nil {
			if err := edit.SetOTLPHeaderName(strings.TrimSpace(*patch.HeaderName)); err != nil {
				return err
			}
		}
		if patch.HeaderEnv != nil {
			if err := edit.SetOTLPHeaderEnv(strings.TrimSpace(*patch.HeaderEnv)); err != nil {
				return err
			}
		}
		if patch.Enabled != nil {
			edit.Observability.OTLP.Enabled = *patch.Enabled
		}
		if patch.ServiceName != nil {
			edit.Observability.OTLP.ServiceName = strings.TrimSpace(*patch.ServiceName)
		}
		*o = edit.Observability.OTLP
		return nil
	})
	if err != nil {
		if errors.Is(err, config.ErrOTLPEndpointInvalid) || errors.Is(err, config.ErrOTLPEndpointCredentials) ||
			errors.Is(err, config.ErrOTLPHeaderInvalid) || errors.Is(err, config.ErrOTLPHeaderEnvInvalid) {
			return observabilityFrom(c.WorkspaceRoot(), got), err
		}
		return observabilityFrom(c.WorkspaceRoot(), got), fmt.Errorf("%w: %w", ErrObservabilityUnwritable, err)
	}
	return observabilityFrom(c.WorkspaceRoot(), got), nil
}
