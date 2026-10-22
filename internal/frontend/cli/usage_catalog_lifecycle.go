package cli

import "reasonix/internal/frontend/clisession"

// closeCLIUsageCatalogs closes the process-level usage projections; see
// clisession.CloseUsageCatalogs.
func closeCLIUsageCatalogs() { clisession.CloseUsageCatalogs() }
