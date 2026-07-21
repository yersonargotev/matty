package claudecode

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/localprojection"
)

type ClassicProjectionKind string

const (
	ClassicSkillProjection       ClassicProjectionKind = "skill"
	ClassicInstructionProjection ClassicProjectionKind = "instruction"
	ClassicMCPProjection         ClassicProjectionKind = "mcp"
)

type ClassicSkill struct{ ID, Name, SourcePath string }
type ClassicInstruction struct{ ID, Content string }
type ClassicMCP struct {
	ID, Name, Command string
	Args              []string
	Environment       map[string]string
}
type ClassicDesired struct {
	Skills      []ClassicSkill
	Instruction *ClassicInstruction
	MCP         *ClassicMCP
}
type ClassicGoal string

const (
	ClassicPresent ClassicGoal = "present"
	ClassicAbsent  ClassicGoal = "absent"
)

type ClassicRequest struct {
	Goal    ClassicGoal
	Desired ClassicDesired
}

type ClassicActionView struct {
	ID                  string
	Kind                ClassicProjectionKind
	Target, Description string
	External            bool
}

type ClassicPlan struct {
	owner                        *SurfaceAdapter
	actions                      []capabilitypack.ProjectionAction
	compatibility                Compatibility
	blockers, preserved, pending []string
	ownership                    []OwnershipRecord
}

func (p ClassicPlan) Compatibility() Compatibility { return p.compatibility }
func (p ClassicPlan) Actions() []ClassicActionView {
	out := make([]ClassicActionView, 0, len(p.actions))
	for _, x := range p.actions {
		out = append(out, ClassicActionView{ID: x.ID, Kind: classicKind(x.Kind), Target: x.Target, Description: x.Description, External: x.Kind == ActionUserMCP})
	}
	return out
}
func (p ClassicPlan) Blockers() []string                  { return append([]string(nil), p.blockers...) }
func (p ClassicPlan) Preserved() []string                 { return append([]string(nil), p.preserved...) }
func (p ClassicPlan) PendingPrerequisites() []string      { return append([]string(nil), p.pending...) }
func (p ClassicPlan) DesiredOwnership() []OwnershipRecord { return cloneOwnership(p.ownership) }

type ClassicApplyResult struct {
	Completed         []string
	Failed            string
	NotStarted        []string
	VerifiedOwnership []OwnershipRecord
	Attempted         bool
}

var ErrForeignClassicPlan = errors.New("classic Claude plan was not inspected by this adapter")

// InspectClassic is inert. It builds classic host projections without exposing
// capability-pack lifecycle values and seals the resulting plan to this adapter.
func (a *SurfaceAdapter) InspectClassic(ctx context.Context, request ClassicRequest) (ClassicPlan, error) {
	if request.Goal != "" && request.Goal != ClassicPresent && request.Goal != ClassicAbsent {
		return ClassicPlan{}, fmt.Errorf("unsupported classic Claude goal %q", request.Goal)
	}
	p := ClassicPlan{owner: a, compatibility: ClassifyVersion(ObserveVersion(ctx, a.executable, a.runner))}
	snapshot := OwnershipSnapshot{}
	if a.ownership != nil {
		var err error
		snapshot, err = a.ownership.ObserveOwnership(ctx)
		if err != nil {
			return ClassicPlan{}, fmt.Errorf("observe Claude ownership: %w", err)
		}
	}
	if request.Goal == ClassicAbsent {
		return a.inspectClassicAbsent(request.Desired, snapshot, p)
	}

	skills := append([]ClassicSkill(nil), request.Desired.Skills...)
	sort.SliceStable(skills, func(i, j int) bool { return skills[i].ID < skills[j].ID })
	seen := map[string]bool{}
	for _, s := range skills {
		id := s.ID
		if id == "" {
			id = "classic:skill:" + s.Name
		}
		target := filepath.Join(a.layout.SkillsDir, s.Name)
		if seen[id] || s.Name == "" || !directChild(target, a.layout.SkillsDir) {
			p.blockers = append(p.blockers, "invalid or duplicate classic Claude skill "+id)
			continue
		}
		seen[id] = true
		source, err := canonicalPath(s.SourcePath)
		if err != nil {
			return ClassicPlan{}, fmt.Errorf("inspect classic Claude skill %s: %w", id, err)
		}
		fp, err := localprojection.FingerprintTree(source)
		if err != nil {
			return ClassicPlan{}, err
		}
		o := ObserveSkill(target, source)
		if o.Err != nil {
			return ClassicPlan{}, o.Err
		}
		record := OwnershipRecord{StateOwner: "classic", ContributorID: "classic", ID: id, Kind: string(ActionSkillLink), Target: target, Fingerprint: fp, Contributors: []string{"classic"}, DeletionAuthorized: true, Skill: SkillIdentity{Surface: "claude", ProjectionID: id, Path: target, SymlinkType: "directory", ResolvedTarget: source, ExpectedSource: source, SourceTreeFingerprint: fp}}
		if o.Kind != PathMissing && !ownsSkillExact(snapshot, id, target, source, o) {
			p.blockers = append(p.blockers, "foreign or drifted Claude skill at "+target)
			p.preserved = append(p.preserved, target)
			continue
		}
		p.ownership = append(p.ownership, record)
		if o.Kind == PathMissing {
			p.actions = append(p.actions, capabilitypack.ProjectionAction{ID: id, Kind: ActionSkillLink, Source: source, Target: target, Description: "link classic Claude skill " + s.Name})
		}
	}
	if d := request.Desired.Instruction; d != nil {
		id := d.ID
		if id == "" {
			id = "classic:instruction"
		}
		current, err := readOptional(a.layout.InstructionsFile)
		if err != nil {
			return ClassicPlan{}, err
		}
		merged, err := UpsertInstructionContribution(string(current), InstructionContribution{ContributorID: "classic", Content: d.Content})
		if err != nil {
			p.blockers = append(p.blockers, err.Error())
			p.preserved = append(p.preserved, a.layout.InstructionsFile)
		} else {
			fp := Fingerprint([]byte(strings.TrimSpace(d.Content)))
			o := ObserveInstructions(a.layout.InstructionsFile)
			if o.Err != nil {
				return ClassicPlan{}, o.Err
			}
			record := OwnershipRecord{StateOwner: "classic", ContributorID: "classic", ID: id, Kind: string(ActionInstructionContribution), Target: a.layout.InstructionsFile, Fingerprint: fp, Contributors: []string{"classic"}, DeletionAuthorized: true}
			observed, exists := o.Contributions["classic"]
			if exists && !ownsClassicFingerprint(snapshot, record, observed) {
				p.blockers = append(p.blockers, "foreign or drifted classic Claude instruction contribution")
				p.preserved = append(p.preserved, a.layout.InstructionsFile)
			} else {
				p.ownership = append(p.ownership, record)
			}
			if !exists || (ownsClassicFingerprint(snapshot, record, observed) && !fingerprintsEqual(observed, fp)) {
				p.actions = append(p.actions, capabilitypack.ProjectionAction{ID: id, Kind: ActionInstructionContribution, Target: a.layout.InstructionsFile, Content: merged, Command: Fingerprint(current), Description: "merge classic Claude instructions"})
			}
		}
	}
	if d := request.Desired.MCP; d != nil {
		id := d.ID
		if id == "" {
			id = "classic:mcp:" + d.Name
		}
		identity := NewMCPIdentity(d.Name, d.Command, d.Args, d.Environment)
		fp := canonicalFingerprint(identity)
		record := OwnershipRecord{StateOwner: "classic", ContributorID: "classic", ID: id, Kind: string(ActionUserMCP), Target: d.Name, Fingerprint: fp, Contributors: []string{"classic"}, DeletionAuthorized: true, Command: d.Command, Args: append([]string(nil), d.Args...), EnvironmentKeys: append([]string(nil), identity.EnvironmentKeys...), EnvironmentFingerprint: identity.EnvironmentFingerprint}
		o := ObserveUserMCP(a.layout.UserMCPFile, d.Name)
		if o.Err != nil {
			p.blockers = append(p.blockers, "Claude user MCP definition is unreadable")
			p.preserved = append(p.preserved, d.Name)
		} else if o.Present && (!fingerprintsEqual(o.DefinitionFingerprint, fp) || !ownsClassicExact(snapshot, record)) {
			p.blockers = append(p.blockers, "foreign or drifted Claude user MCP "+d.Name)
			p.preserved = append(p.preserved, d.Name)
		} else {
			p.ownership = append(p.ownership, record)
			if !o.Present {
				if p.compatibility != CompatibilitySupported {
					p.pending = append(p.pending, p.compatibility.Remediation())
				} else {
					args := []string{"mcp", "add", d.Name, "--scope", "user"}
					for _, key := range identity.EnvironmentKeys {
						args = append(args, "--env", key+"="+d.Environment[key])
					}
					args = append(args, "--", d.Command)
					args = append(args, d.Args...)
					p.actions = append(p.actions, capabilitypack.ProjectionAction{ID: id, Kind: ActionUserMCP, Target: d.Name, Command: a.executable, Args: args, Content: fp, Description: "configure redacted classic Claude user MCP " + d.Name})
				}
			}
		}
	}
	p.actions = localBeforeMCP(p.actions)
	return p, nil
}

func (a *SurfaceAdapter) inspectClassicAbsent(desired ClassicDesired, s OwnershipSnapshot, p ClassicPlan) (ClassicPlan, error) {
	wanted := map[string]bool{}
	for _, x := range desired.Skills {
		wanted[x.ID] = true
	}
	if desired.Instruction != nil {
		wanted[desired.Instruction.ID] = true
	}
	if desired.MCP != nil {
		wanted[desired.MCP.ID] = true
	}
	for _, r := range s.Records {
		if r.StateOwner != "classic" || r.ContributorID != "classic" || !slicesContains(r.Contributors, "classic") || len(wanted) > 0 && !wanted[r.ID] {
			continue
		}
		var x capabilitypack.ProjectionAction
		switch r.Kind {
		case string(ActionSkillLink):
			o := ObserveSkill(r.Target, r.Skill.ExpectedSource)
			if o.Err != nil {
				return p, o.Err
			}
			if o.Kind == PathMissing {
				continue
			}
			if !r.MatchesSkill("claude", r.ID, r.Target, r.Skill.ExpectedSource, o) || !r.DeletionAuthorized || len(r.Contributors) > 1 {
				p.blockers = append(p.blockers, "owned Claude skill changed or shared; preserving it")
				p.preserved = append(p.preserved, r.Target)
				continue
			}
			x = capabilitypack.ProjectionAction{ID: r.ID, Kind: ActionSkillLink, Target: r.Target, Mode: capabilitypack.ProjectionDeleteTarget, Description: "remove classic Claude skill"}
		case string(ActionInstructionContribution):
			o := ObserveInstructions(r.Target)
			if o.Err != nil {
				p.blockers = append(p.blockers, "owned Claude instructions unreadable; preserving them")
				p.preserved = append(p.preserved, r.Target)
				continue
			}
			fp, ok := o.Contributions["classic"]
			if !ok {
				continue
			}
			if !fingerprintsEqual(fp, r.Fingerprint) {
				p.blockers = append(p.blockers, "owned Claude instruction changed; preserving it")
				p.preserved = append(p.preserved, r.Target)
				continue
			}
			current, _ := readOptional(r.Target)
			merged, err := RemoveInstructionContribution(string(current), "classic")
			if err != nil {
				return p, err
			}
			x = capabilitypack.ProjectionAction{ID: r.ID, Kind: ActionInstructionContribution, Target: r.Target, Content: merged, Command: Fingerprint(current), Mode: capabilitypack.ProjectionRemoveContent, Description: "remove classic Claude instructions"}
		case string(ActionUserMCP):
			o := ObserveUserMCP(a.layout.UserMCPFile, r.Target)
			if o.Err != nil {
				p.blockers = append(p.blockers, "owned Claude user MCP unreadable; preserving it")
				p.preserved = append(p.preserved, r.Target)
				continue
			}
			if !o.Present {
				continue
			}
			if !fingerprintsEqual(o.DefinitionFingerprint, r.Fingerprint) || !r.DeletionAuthorized || len(r.Contributors) > 1 {
				p.blockers = append(p.blockers, "owned Claude user MCP changed or shared; preserving it")
				p.preserved = append(p.preserved, r.Target)
				continue
			}
			if p.compatibility != CompatibilitySupported {
				p.pending = append(p.pending, p.compatibility.Remediation())
				p.preserved = append(p.preserved, r.Target)
				continue
			}
			x = capabilitypack.ProjectionAction{ID: r.ID, Kind: ActionUserMCP, Target: r.Target, Command: a.executable, Args: []string{"mcp", "remove", r.Target, "--scope", "user"}, Mode: capabilitypack.ProjectionDeleteTarget, Description: "remove redacted classic Claude user MCP " + r.Target}
		default:
			continue
		}
		p.actions = append(p.actions, x)
	}
	p.actions = localBeforeMCP(p.actions)
	return p, nil
}

func (a *SurfaceAdapter) ApplyClassic(ctx context.Context, p ClassicPlan) (ClassicApplyResult, error) {
	if p.owner != a {
		return ClassicApplyResult{}, ErrForeignClassicPlan
	}
	r := ClassicApplyResult{NotStarted: actionIDs(p.actions)}
	if len(p.actions) == 0 {
		return r, nil
	}
	if err := a.validateActions(p.actions); err != nil {
		return r, err
	}
	unlock, err := a.lock()
	if err != nil {
		return r, err
	}
	defer unlock()
	snapshot := OwnershipSnapshot{}
	if a.ownership != nil {
		snapshot, err = a.ownership.ObserveOwnership(ctx)
		if err != nil {
			return r, fmt.Errorf("observe fresh Claude ownership: %w", err)
		}
	}
	if _, err := a.preflight(p.actions, snapshot); err != nil {
		return r, err
	}
	for i, action := range p.actions {
		r.Attempted = true
		if err := a.apply(ctx, action); err != nil {
			r.Failed = action.ID
			r.NotStarted = actionIDs(p.actions[i+1:])
			return r, err
		}
		r.Completed = append(r.Completed, action.ID)
		for _, record := range p.ownership {
			if record.ID == action.ID {
				r.VerifiedOwnership = append(r.VerifiedOwnership, cloneOwnership([]OwnershipRecord{record})...)
			}
		}
	}
	r.NotStarted = nil
	r.VerifiedOwnership = cloneOwnership(p.ownership)
	return r, nil
}

func ownsClassicExact(snapshot OwnershipSnapshot, want OwnershipRecord) bool {
	return ownsClassicFingerprint(snapshot, want, want.Fingerprint)
}

func ownsClassicFingerprint(snapshot OwnershipSnapshot, want OwnershipRecord, fingerprint string) bool {
	matches := 0
	for _, record := range snapshot.Records {
		if record.StateOwner == "classic" && record.ContributorID == "classic" &&
			record.ID == want.ID && record.Kind == want.Kind &&
			filepath.Clean(record.Target) == filepath.Clean(want.Target) &&
			fingerprintsEqual(record.Fingerprint, fingerprint) &&
			slicesContains(record.Contributors, "classic") {
			matches++
		}
	}
	return matches == 1
}

func localBeforeMCP(in []capabilitypack.ProjectionAction) []capabilitypack.ProjectionAction {
	out := make([]capabilitypack.ProjectionAction, 0, len(in))
	for _, x := range in {
		if x.Kind != ActionUserMCP {
			out = append(out, x)
		}
	}
	for _, x := range in {
		if x.Kind == ActionUserMCP {
			out = append(out, x)
		}
	}
	return out
}
func actionIDs(in []capabilitypack.ProjectionAction) []string {
	out := make([]string, len(in))
	for i := range in {
		out[i] = in[i].ID
	}
	return out
}
func classicKind(k capabilitypack.ProjectionActionKind) ClassicProjectionKind {
	switch k {
	case ActionSkillLink:
		return ClassicSkillProjection
	case ActionInstructionContribution:
		return ClassicInstructionProjection
	default:
		return ClassicMCPProjection
	}
}
func cloneOwnership(in []OwnershipRecord) []OwnershipRecord {
	out := append([]OwnershipRecord(nil), in...)
	for i := range out {
		out[i].Contributors = append([]string(nil), out[i].Contributors...)
		out[i].Args = append([]string(nil), out[i].Args...)
		out[i].EnvironmentKeys = append([]string(nil), out[i].EnvironmentKeys...)
	}
	return out
}
func slicesContains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
