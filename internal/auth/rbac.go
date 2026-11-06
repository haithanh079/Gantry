package auth

import "sync"

// RoleData is a DTO for loading role definitions into the in-memory store.
type RoleData struct {
	Name        string
	Level       int
	Permissions map[string]bool
}

// roleStore is a thread-safe cache of role definitions, loaded from the DB
// on startup and refreshed after any role mutation.
type roleStore struct {
	mu          sync.RWMutex
	hierarchy   map[string]int             // role name → level
	permissions map[string]map[string]bool // role name → action → allowed
}

var globalStore = &roleStore{
	// Seed with defaults so the system works before DB is loaded.
	hierarchy: map[string]int{
		"viewer": 1, "developer": 2, "platform-engineer": 3, "admin": 4,
	},
	permissions: map[string]map[string]bool{
		"viewer":            {"read": true, "write": false, "execute": false, "delete": false, "admin": false},
		"developer":         {"read": true, "write": true, "execute": true, "delete": true, "admin": false},
		"platform-engineer": {"read": true, "write": true, "execute": true, "delete": true, "admin": false},
		"admin":             {"read": true, "write": true, "execute": true, "delete": true, "admin": true},
	},
}

// InitRoleStore replaces the in-memory role cache with the given roles.
// Called on startup after loading from DB, and after any role CRUD operation.
func InitRoleStore(roles []RoleData) {
	h := make(map[string]int, len(roles))
	p := make(map[string]map[string]bool, len(roles))
	for _, r := range roles {
		h[r.Name] = r.Level
		perms := make(map[string]bool, len(r.Permissions))
		for k, v := range r.Permissions {
			perms[k] = v
		}
		p[r.Name] = perms
	}

	globalStore.mu.Lock()
	globalStore.hierarchy = h
	globalStore.permissions = p
	globalStore.mu.Unlock()
}

// RoleLevel returns the hierarchy level for a role (0 if unknown).
func RoleLevel(role string) int {
	globalStore.mu.RLock()
	defer globalStore.mu.RUnlock()
	return globalStore.hierarchy[role]
}

// RolePermissions returns a copy of the permissions map for a role.
// Returns nil if the role is unknown.
func RolePermissions(role string) map[string]bool {
	globalStore.mu.RLock()
	perms, ok := globalStore.permissions[role]
	globalStore.mu.RUnlock()
	if !ok {
		return nil
	}
	result := make(map[string]bool, len(perms))
	for k, v := range perms {
		result[k] = v
	}
	return result
}

// IsValidRole checks if a role exists in the store.
func IsValidRole(role string) bool {
	globalStore.mu.RLock()
	defer globalStore.mu.RUnlock()
	_, ok := globalStore.hierarchy[role]
	return ok
}

// AllRoleLevels returns a copy of the role hierarchy map.
func AllRoleLevels() map[string]int {
	globalStore.mu.RLock()
	defer globalStore.mu.RUnlock()
	result := make(map[string]int, len(globalStore.hierarchy))
	for k, v := range globalStore.hierarchy {
		result[k] = v
	}
	return result
}

// GroupRole is a minimal interface for group role lookup.
type GroupRole interface {
	GetRole() string
}

// EffectiveRole returns the highest role between the user's direct role
// and all their group roles. This is backwards-compatible: if the user
// has no groups, their direct role is returned unchanged.
func EffectiveRole(userRole string, groupRoles []string) string {
	best := userRole
	bestLevel := RoleLevel(best)
	for _, gr := range groupRoles {
		if lvl := RoleLevel(gr); lvl > bestLevel {
			best = gr
			bestLevel = lvl
		}
	}
	return best
}

// PermissionCheck represents the fields needed to evaluate a permission rule.
type PermissionCheck struct {
	SubjectType    string // "user" or "group"
	SubjectID      string
	ResourceType   string // "entity", "action", "plugin", "landscape", "observed", "*"
	ResourceFilter string // kind, namespace, or ""
	Action         string // "read", "write", "delete", "execute", "admin", "*"
	Effect         string // "allow" or "deny"
}

// EvaluateAccess checks whether an action is allowed given the effective role
// and a set of applicable permission rules.
//
// Logic:
//  1. If no rules exist → fall through to role-based check (backwards compat)
//  2. Any matching deny rule → denied
//  3. Any matching allow rule → allowed
//  4. Fall back to role hierarchy
func EvaluateAccess(effectiveRole string, rules []PermissionCheck, resourceType, resourceFilter, action string) bool {
	if len(rules) == 0 {
		return roleAllows(effectiveRole, action)
	}

	hasDeny := false
	hasAllow := false

	for _, r := range rules {
		if !ruleMatches(r, resourceType, resourceFilter, action) {
			continue
		}
		if r.Effect == "deny" {
			hasDeny = true
		} else if r.Effect == "allow" {
			hasAllow = true
		}
	}

	// Deny always wins.
	if hasDeny {
		return false
	}
	if hasAllow {
		return true
	}

	// No matching rules — fall back to role hierarchy.
	return roleAllows(effectiveRole, action)
}

// ruleMatches checks if a permission rule applies to the given resource and action.
func ruleMatches(r PermissionCheck, resourceType, resourceFilter, action string) bool {
	if r.ResourceType != "*" && r.ResourceType != resourceType {
		return false
	}
	if r.ResourceFilter != "" && r.ResourceFilter != "*" && r.ResourceFilter != resourceFilter {
		return false
	}
	if r.Action != "*" && r.Action != action {
		return false
	}
	return true
}

// roleAllows checks if the role's configured permissions permit the given action.
// Reads from the in-memory role store.
func roleAllows(role, action string) bool {
	globalStore.mu.RLock()
	perms, ok := globalStore.permissions[role]
	globalStore.mu.RUnlock()

	if !ok {
		// Unknown role — fall back to level-based check for safety.
		level := RoleLevel(role)
		switch action {
		case "read":
			return level >= 1
		case "write", "execute", "delete":
			return level >= 2
		case "admin", "*":
			return level >= 4
		default:
			return level >= 1
		}
	}

	// Wildcard action: only allowed if all permissions are granted.
	if action == "*" {
		return perms["admin"]
	}

	return perms[action]
}
