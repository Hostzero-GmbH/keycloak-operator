package controller

import (
	"context"
	"errors"
	"fmt"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
	"github.com/Hostzero-GmbH/keycloak-operator/internal/keycloak"
)

// errTokenExchangeWaiting is returned by reconcileTokenExchange when the
// reconcile cannot proceed because some referenced Keycloak resource (today:
// one of the `allowedClients`) isn't there yet. The caller treats it as a
// soft condition: requeue without logging at ERROR level + surface a friendly
// message on status. The race resolves itself once the referenced resources
// (typically siblings being applied in the same wave) reconcile to Ready.
var errTokenExchangeWaiting = errors.New("token-exchange reconcile waiting on referenced Keycloak state")

// IsTokenExchangeWaiting reports whether the given error is the "waiting on
// referenced state" sentinel. Exposed for the parent IdP controller's caller
// path; tests cover both code paths via this helper.
func IsTokenExchangeWaiting(err error) bool { return errors.Is(err, errTokenExchangeWaiting) }

// reconcileTokenExchange wires Keycloak fine-grained authz so that exactly the
// listed clients may use this IdP as `subject_issuer` in an RFC 8693 token
// exchange. Mechanics:
//
//  1. Enable management permissions on the IdP (Keycloak auto-creates a
//     scope-permission named "token-exchange.permission.idp.<internalId>" on
//     the realm-management authz resource server).
//  2. Resolve realm-management client UUID + each allowed clientId → UUID.
//  3. Ensure a Client-type authz policy named
//     "hostzero-idp-<alias>-token-exchange" exists in realm-management's
//     resource server, with `clients` = the resolved UUIDs.
//  4. Bind that policy to the auto-created scope-permission (Policies list
//     becomes exactly [policyID] — the operator owns this binding).
//
// The reverse (delete) path is in tokenExchangeCleanup below.
func (r *KeycloakIdentityProviderReconciler) reconcileTokenExchange(
	ctx context.Context,
	kc *keycloak.Client,
	realmName, alias string,
	idp *keycloakv1beta1.KeycloakIdentityProvider,
) (*keycloakv1beta1.IDPTokenExchangeStatus, error) {
	spec := idp.Spec.TokenExchange
	if spec == nil {
		return nil, nil
	}

	// 1. Enable IdP permissions; pull the token-exchange scope-permission ID.
	ref, err := kc.SetIdentityProviderPermissionsEnabled(ctx, realmName, alias, true)
	if err != nil {
		return nil, fmt.Errorf("enable IdP permissions: %w", err)
	}
	permID := ref.ScopePermissions["token-exchange"]
	if permID == "" {
		return nil, fmt.Errorf("Keycloak returned no token-exchange scope permission for IdP %q (got %v)", alias, ref.ScopePermissions)
	}

	// 2. realm-management client UUID (the authz resource server).
	realmMgmt, err := kc.GetClientByClientID(ctx, realmName, "realm-management")
	if err != nil {
		return nil, fmt.Errorf("look up realm-management client: %w", err)
	}
	if realmMgmt == nil || realmMgmt.ID == nil {
		return nil, fmt.Errorf("realm-management client not found in realm %q", realmName)
	}
	rmgmtUUID := *realmMgmt.ID

	// 3. Resolve the desired client UUIDs from the spec list.
	//
	// A missing referenced client is a soft "waiting" condition (not a hard
	// error). On first apply, siblings in the same ArgoCD/Helm wave race —
	// the IdP-with-tokenExchange and its referenced clients are both at
	// default wave 0, so the IdP can reconcile before the clients exist.
	// We return errTokenExchangeWaiting so the parent controller can
	// requeue with a friendly status message instead of logging ERROR.
	allowedUUIDs := make([]string, 0, len(spec.AllowedClients))
	for _, clientID := range spec.AllowedClients {
		c, err := kc.GetClientByClientID(ctx, realmName, clientID)
		if err != nil {
			return nil, fmt.Errorf("resolve allowed client %q: %w", clientID, err)
		}
		if c == nil || c.ID == nil {
			return nil, fmt.Errorf("%w: client %q not yet present in realm %q", errTokenExchangeWaiting, clientID, realmName)
		}
		allowedUUIDs = append(allowedUUIDs, *c.ID)
	}

	// 4. Find or create the managed policy.
	policyName := fmt.Sprintf("hostzero-idp-%s-token-exchange", alias)
	desired := keycloak.ClientPolicyRepresentation{
		Name:        policyName,
		Description: fmt.Sprintf("Managed by hostzero-keycloak-operator; allowed token-exchange clients for IdP %q", alias),
		// Type is implicit on the URL path (/policy/client) for POST, but the
		// type-specific GET on /policy/client/{id} populates Type="client".
		// Set it explicitly so clientPolicyEqual stays symmetric and the
		// stored representation has a consistent Type whichever API was last
		// used to write it.
		Type:             "client",
		Logic:            "POSITIVE",
		DecisionStrategy: "UNANIMOUS",
		Clients:          allowedUUIDs,
	}

	existing, err := kc.SearchClientPolicyByName(ctx, realmName, rmgmtUUID, policyName)
	if err != nil {
		return nil, fmt.Errorf("search managed policy: %w", err)
	}

	var policyID string
	if existing == nil {
		id, err := kc.CreateClientPolicy(ctx, realmName, rmgmtUUID, desired)
		if err != nil {
			return nil, fmt.Errorf("create managed policy: %w", err)
		}
		policyID = id
	} else {
		policyID = existing.ID
		desired.ID = policyID
		if !clientPolicyEqual(existing, &desired) {
			if err := kc.UpdateClientPolicy(ctx, realmName, rmgmtUUID, policyID, desired); err != nil {
				return nil, fmt.Errorf("update managed policy: %w", err)
			}
		}
	}

	// 5. Bind the policy to the scope-permission.
	perm, err := kc.GetScopePermission(ctx, realmName, rmgmtUUID, permID)
	if err != nil {
		return nil, fmt.Errorf("get scope permission %s: %w", permID, err)
	}
	if !permissionPoliciesMatch(perm.Policies, []string{policyID}) {
		perm.Policies = []string{policyID}
		if err := kc.UpdateScopePermission(ctx, realmName, rmgmtUUID, permID, *perm); err != nil {
			return nil, fmt.Errorf("bind policy to scope permission: %w", err)
		}
	}

	return &keycloakv1beta1.IDPTokenExchangeStatus{
		Enabled:      true,
		PermissionID: permID,
		PolicyID:     policyID,
		PolicyName:   policyName,
	}, nil
}

// cleanupTokenExchange removes the operator-managed policy on IdP deletion.
// Disabling IdP permissions is implicit on IdP delete (Keycloak removes the
// scope-permission), but the policy in realm-management's resource server
// would otherwise stay as an orphan.
//
// Called with best-effort semantics: errors are logged by the caller but
// don't block IdP deletion (the IdP is going away anyway).
func (r *KeycloakIdentityProviderReconciler) cleanupTokenExchange(
	ctx context.Context,
	kc *keycloak.Client,
	realmName string,
	idp *keycloakv1beta1.KeycloakIdentityProvider,
) error {
	if idp.Status.TokenExchange == nil || idp.Status.TokenExchange.PolicyID == "" {
		return nil
	}

	realmMgmt, err := kc.GetClientByClientID(ctx, realmName, "realm-management")
	if err != nil {
		return fmt.Errorf("look up realm-management client: %w", err)
	}
	if realmMgmt == nil || realmMgmt.ID == nil {
		return nil
	}

	if err := kc.DeletePolicy(ctx, realmName, *realmMgmt.ID, idp.Status.TokenExchange.PolicyID); err != nil {
		return fmt.Errorf("delete managed policy %s: %w", idp.Status.TokenExchange.PolicyID, err)
	}
	return nil
}

// clientPolicyEqual returns true if two ClientPolicyRepresentations carry the
// same operator-managed fields (name/description/logic/decisionStrategy +
// client-UUID set). Used to skip unnecessary PUTs on every reconcile.
func clientPolicyEqual(a, b *keycloak.ClientPolicyRepresentation) bool {
	if a.Name != b.Name || a.Description != b.Description ||
		a.Logic != b.Logic || a.DecisionStrategy != b.DecisionStrategy {
		return false
	}
	return stringSetsEqual(a.Clients, b.Clients)
}

// permissionPoliciesMatch returns true if the two policy-ID lists describe
// the same set.
func permissionPoliciesMatch(have, want []string) bool {
	return stringSetsEqual(have, want)
}

// stringSetsEqual treats both slices as sets and compares membership.
func stringSetsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]struct{}, len(a))
	for _, s := range a {
		seen[s] = struct{}{}
	}
	for _, s := range b {
		if _, ok := seen[s]; !ok {
			return false
		}
	}
	return true
}
