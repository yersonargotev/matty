#!/usr/bin/env bash

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

run_row() {
  local row="$1" package="$2" tests="$3"
  echo "==> Addy acceptance row $row"
  local listed
  listed="$(go test "$package" -list "^(${tests})$")"
  if ! grep -q '^Test' <<<"$listed"; then
    echo "row $row references no executable test in $package: $tests" >&2
    return 1
  fi
  go test "$package" -run "^(${tests})$" -count=1
}

# Each row executes its owning Packy subsystem. The Addy-specific cohort tests
# are paired with the pre-existing engine negative that proves the same gate's
# effect boundary; fixture predicates are supplemental diagnostics only.
run_row 1 ./internal/addyacceptance 'TestExactUpstreamArchiveInventoryAndSupportRemainInert'
run_row 2 ./internal/addyacceptance 'TestUnsafeArchiveTwinBlocksAndCleansBeforeExecution|TestExactUpstreamArchiveInventoryAndSupportRemainInert'
run_row 4 ./internal/packsync 'TestLoadConfigRejectsPathUnsafeSourceIDsAndSharedBindings|TestValidatePreconditionsRejectsUnrelatedSourceGenerationWithoutMutation'
run_row 6 ./internal/addyacceptance 'TestCanonicalInventoryAndDeterminism|TestOneFactNegativeTwinBlocksCompleteInventory'
run_row 7 ./internal/capabilitypack 'TestDiscoverRejectsInvalidManifestV2Contracts|TestCompleteAddyCohortUsesTypedConsentFreshVerificationAndExactNoOp'
run_row 8 ./internal/addyacceptance 'TestExactUpstreamArchiveInventoryAndSupportRemainInert|TestCompleteSurfaceCohortsAreDeterministicInertAndIndependent'
run_row 9 ./internal/ci 'TestPackSourceV2SchemasAcceptCanonicalRuntimeContracts|TestSynchronizationSchemasAcceptCanonicalRuntimeArtifacts'
run_row 10 ./internal/packclassification 'TestHumanClassificationRequiresInspectionThenBoundEvidenceDispatch'
run_row 11 ./internal/addyacceptance 'TestLifecycleOracleExposesExactCountsAuthoritiesAndSurfaceBindings'
run_row 12 ./internal/addyacceptance 'TestCompleteSurfaceCohortsAreDeterministicInertAndIndependent'
run_row 13 ./internal/addyacceptance 'TestCompleteSurfaceCohortsAreDeterministicInertAndIndependent'
run_row 14 ./internal/capabilitypack 'TestCompleteAddyCollisionBlocksUntilExactSurfaceAliasReplans'
run_row 15 ./internal/capabilitypack 'TestCompleteAddyCohortStalePreflightAndAtomicFailureRequireFreshRecovery'
run_row 16 ./internal/capabilitypack 'TestCompleteAddyDualSurfaceFailurePreservesAuthorizedOtherSurface|TestCompleteAddyAliasesRemainSurfaceLocalAndSharedRemovalRetainsContributor'
run_row 17 ./internal/capabilitypack 'TestCompleteAddyCohortUsesTypedConsentFreshVerificationAndExactNoOp'
run_row 18 ./internal/capabilitypack 'TestCompleteAddyAtomicAdapterFailureRecordsAttemptAndRequiresFreshRecoveryPlan'
run_row 19 ./internal/capabilitypack 'TestCompleteAddyReadinessKeepsUnknownPendingOptionalAndExcludedDistinct'
run_row 19 ./internal/cli 'TestPackStatusJSONRequireEmitsDocumentBeforeGateError|TestPackStatusRequireUsableIsIndependentNonInteractiveGate'
run_row 20 ./internal/capabilitypack 'TestCompleteAddyReadinessKeepsUnknownPendingOptionalAndExcludedDistinct'
run_row 21 ./internal/capabilitypack 'TestCompleteAddyCohortUsesTypedConsentFreshVerificationAndExactNoOp|TestUpdateRejectsStaleCatalogAndExactPlanApproval'
run_row 22 ./internal/capabilitypack 'TestCompleteAddyExactOwnershipRemovalBlocksDriftWithoutEffects|TestCompleteAddyAliasesRemainSurfaceLocalAndSharedRemovalRetainsContributor'
run_row 23 ./internal/tools/syncpacksource 'TestSandboxTracerRunsInspectClassifyValidatePublishWithoutExternalWrites|TestInspectCarriesStrictRegistrationIntoCheck'
run_row 23 ./internal/packsync 'TestCheckSealsAbsentSourceRegistrationWithoutPersistingIt|TestApplyCommitsRegistrationConfigurationLockAndContributionAtomically'
run_row 24 ./internal/packsync 'TestCheckRejectsRegistrationWithExistingSourceOrBindingOwner|TestApplyCommitsRegistrationConfigurationLockAndContributionAtomically'
run_row 24 ./internal/ci 'TestPackSourceV2RegistrationSemanticAndNullArrayValidation|TestSyncWorkflowIsManualPinnedLeastPrivilegeAndPhaseSeparated'
