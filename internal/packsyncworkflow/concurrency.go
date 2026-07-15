package packsyncworkflow

type QueuedRequest struct {
	Identity        string
	NeedsFreshCheck bool
}

type ConcurrencyState struct {
	Active  *QueuedRequest
	Pending *QueuedRequest
}

type Admission string

const (
	AdmissionActive     Admission = "active"
	AdmissionPending    Admission = "pending"
	AdmissionSuperseded Admission = "superseded-pending"
	AdmissionAttached   Admission = "attached"
)

// Admit models GitHub's one-active/one-pending concurrency contract without
// canceling the active request. Identical requests attach; a distinct newer
// request replaces only the pending request.
func (state *ConcurrencyState) Admit(identity string) Admission {
	if state.Active == nil {
		state.Active = &QueuedRequest{Identity: identity, NeedsFreshCheck: true}
		return AdmissionActive
	}
	if state.Active.Identity == identity || state.Pending != nil && state.Pending.Identity == identity {
		return AdmissionAttached
	}
	request := &QueuedRequest{Identity: identity, NeedsFreshCheck: true}
	if state.Pending == nil {
		state.Pending = request
		return AdmissionPending
	}
	state.Pending = request
	return AdmissionSuperseded
}

func (state *ConcurrencyState) CompleteActive() *QueuedRequest {
	promoted := state.Pending
	state.Active = promoted
	state.Pending = nil
	if promoted != nil {
		promoted.NeedsFreshCheck = true
	}
	return promoted
}
