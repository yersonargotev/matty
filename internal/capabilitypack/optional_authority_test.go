package capabilitypack

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestSurfaceGatewayCanonicalizesAndDetachesOptionalAuthorities(t *testing.T) {
	authorities := []OptionalAuthorityObservation{
		{ModeID: "research", Authority: "network", State: OptionalAuthorityUnknown, Fallback: "use static evidence"},
		{ModeID: "research", Authority: "browser", State: OptionalAuthorityUnavailable, Fallback: "use static evidence"},
	}
	adapter := &gatewayAdapter{inspection: SurfaceInspection{Readiness: ReadinessObservation{OptionalAuthorities: authorities}}}
	transition := SurfaceTransition{Desired: Pack{ID: "addy", Contract: Contract{OptionalModes: []OptionalMode{{
		ID: "research", Authorities: []string{"browser", "network"}, Fallback: "use static evidence",
	}}}}}

	got, err := inspectSurface(context.Background(), adapter, transition)
	if err != nil {
		t.Fatal(err)
	}
	adapter.inspection.Readiness.OptionalAuthorities[0].Fallback = "mutated"
	want := []OptionalAuthorityObservation{
		{ModeID: "research", Authority: "browser", State: OptionalAuthorityUnavailable, Fallback: "use static evidence"},
		{ModeID: "research", Authority: "network", State: OptionalAuthorityUnknown, Fallback: "use static evidence"},
	}
	if !reflect.DeepEqual(got.Readiness.OptionalAuthorities, want) {
		t.Fatalf("optional authorities = %#v, want %#v", got.Readiness.OptionalAuthorities, want)
	}
}

func TestSurfaceGatewayRejectsInvalidOptionalAuthorities(t *testing.T) {
	mode := OptionalMode{ID: "research", Authorities: []string{"browser"}, Fallback: "use static evidence"}
	transition := SurfaceTransition{Desired: Pack{ID: "addy", Contract: Contract{OptionalModes: []OptionalMode{mode}}}}
	valid := OptionalAuthorityObservation{ModeID: mode.ID, Authority: "browser", State: OptionalAuthorityAvailable, Fallback: mode.Fallback}
	for _, tc := range []struct {
		name string
		got  []OptionalAuthorityObservation
		want string
	}{
		{name: "unsupported state", got: []OptionalAuthorityObservation{{ModeID: mode.ID, Authority: "browser", State: "future", Fallback: mode.Fallback}}, want: "unsupported optional authority state"},
		{name: "duplicate", got: []OptionalAuthorityObservation{valid, valid}, want: "duplicate optional authority"},
		{name: "unknown authority", got: []OptionalAuthorityObservation{{ModeID: mode.ID, Authority: "network", State: OptionalAuthorityAvailable, Fallback: mode.Fallback}}, want: "malformed optional authority"},
		{name: "altered fallback", got: []OptionalAuthorityObservation{{ModeID: mode.ID, Authority: "browser", State: OptionalAuthorityAvailable, Fallback: "different"}}, want: "malformed optional authority"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapter := &gatewayAdapter{inspection: SurfaceInspection{Readiness: ReadinessObservation{OptionalAuthorities: tc.got}}}
			if _, err := inspectSurface(context.Background(), adapter, transition); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestStatusCarriesOptionalAuthoritiesWithoutChangingReadiness(t *testing.T) {
	mode := OptionalMode{ID: "research", Authorities: []string{"browser"}, Fallback: "use static evidence"}
	pack := Pack{
		ID: "addy", Version: "1", Surfaces: []Surface{SurfaceCodex},
		Contract:  Contract{OptionalModes: []OptionalMode{mode}},
		Resources: []Resource{{Kind: "instruction", ID: "guide"}},
	}
	projection := ObservedProjection{Goal: ProjectionPresent, ID: "instruction:guide", Exists: true, ObservedFingerprint: "same", DesiredFingerprint: "same", Action: ProjectionAction{ID: "instruction:guide"}}
	authority := OptionalAuthorityObservation{ModeID: mode.ID, Authority: "browser", State: OptionalAuthorityUnavailable, Fallback: mode.Fallback}
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{{
		Projections: []ObservedProjection{projection},
		Readiness: ReadinessObservation{
			AuthorizationObserved: true, Authorized: true, UsabilityObserved: true, Usable: true,
			OptionalAuthorities: []OptionalAuthorityObservation{authority},
		},
	}}}
	store := &fakeActivationStore{state: ActivationState{Ownership: []ProjectionOwnership{{
		ID: projection.ID, Fingerprint: "same", Contributors: []string{pack.ID},
	}}}}
	facade := NewFacade(Catalog{packs: []Pack{pack}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))

	report, err := facade.Status(context.Background(), StatusRequest{PackID: pack.ID, Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	entry := report.Entries[0]
	if entry.Readiness != (ReadinessStatus{Configured: true, Authorized: true, Usable: true}) {
		t.Fatalf("optional authority changed readiness: %+v", entry.Readiness)
	}
	if !reflect.DeepEqual(entry.OptionalAuthorities, []OptionalAuthorityObservation{authority}) {
		t.Fatalf("optional authorities = %#v", entry.OptionalAuthorities)
	}
}
