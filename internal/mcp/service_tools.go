package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/go2engle/gantry/internal/api/handlers"
	"github.com/go2engle/gantry/internal/api/middleware"
	"github.com/go2engle/gantry/internal/entity"
	"github.com/go2engle/gantry/internal/events"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type createServiceInput struct {
	Name        string   `json:"name" jsonschema:"service name (metadata.name); lowercase slug"`
	Namespace   string   `json:"namespace,omitempty" jsonschema:"namespace; defaults to \"default\""`
	Title       string   `json:"title,omitempty" jsonschema:"human-friendly title"`
	Description string   `json:"description,omitempty" jsonschema:"short description"`
	Owner       string   `json:"owner,omitempty" jsonschema:"owning team name"`
	Type        string   `json:"type,omitempty" jsonschema:"service type: backend, frontend, fullstack, library, or worker"`
	Lifecycle   string   `json:"lifecycle,omitempty" jsonschema:"lifecycle: experimental, development, production, or deprecated"`
	RepoURL     string   `json:"repoUrl,omitempty" jsonschema:"source repository URL"`
	Tags        []string `json:"tags,omitempty" jsonschema:"free-form tags"`
}

type getServiceInput struct {
	Name      string `json:"name" jsonschema:"service name (metadata.name)"`
	Namespace string `json:"namespace,omitempty" jsonschema:"namespace; defaults to \"default\""`
}

type updateServiceInput struct {
	Name        string    `json:"name" jsonschema:"service name (metadata.name)"`
	Namespace   string    `json:"namespace,omitempty" jsonschema:"namespace; defaults to \"default\""`
	Title       *string   `json:"title,omitempty" jsonschema:"replace metadata.title when set"`
	Description *string   `json:"description,omitempty" jsonschema:"replace metadata.description when set"`
	Owner       *string   `json:"owner,omitempty" jsonschema:"replace metadata.owner when set"`
	Type        *string   `json:"type,omitempty" jsonschema:"replace spec.type when set"`
	Lifecycle   *string   `json:"lifecycle,omitempty" jsonschema:"replace spec.lifecycle when set"`
	RepoURL     *string   `json:"repoUrl,omitempty" jsonschema:"replace spec.repoUrl when set"`
	Tags        *[]string `json:"tags,omitempty" jsonschema:"replace metadata.tags when set"`
}

type linkServiceInput struct {
	Name       string `json:"name" jsonschema:"service to update (metadata.name)"`
	Namespace  string `json:"namespace,omitempty" jsonschema:"namespace of the service; defaults to \"default\""`
	Target     string `json:"target" jsonschema:"relation or url"`
	Op         string `json:"op" jsonschema:"add or remove"`
	Relation   string `json:"relation,omitempty" jsonschema:"when target is relation: dependsOn, providesApi, consumesApi, deployedIn, or owner"`
	Kind       string `json:"kind,omitempty" jsonschema:"related entity kind. Required for dependsOn and deployedIn. owner expects Team. providesApi and consumesApi expect API"`
	TargetName string `json:"targetName,omitempty" jsonschema:"related entity name, or the team name when relation is owner"`
	Title      string `json:"title,omitempty" jsonschema:"link title when target is url"`
	URL        string `json:"url,omitempty" jsonschema:"link URL when target is url"`
	Icon       string `json:"icon,omitempty" jsonschema:"optional link icon: dashboard, docs, runbook, github, slack, alert, monitor, ci, or other"`
}

var linkIcons = map[string]bool{
	"dashboard": true,
	"docs":      true,
	"runbook":   true,
	"github":    true,
	"slack":     true,
	"alert":     true,
	"monitor":   true,
	"ci":        true,
	"other":     true,
}

func registerServiceTools(srv *mcpsdk.Server, h *handlers.Handlers) {
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "create_service",
		Description: "Create a Service entity. Requires developer role or higher. Sets kind to Service, validates the schema, writes an audit entry, and triggers GitOps when enabled. Call get_service afterwards if you need the stored record.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in createServiceInput) (*mcpsdk.CallToolResult, any, error) {
		if err := requireDeveloperRole(ctx); err != nil {
			return nil, nil, err
		}
		e := entity.Entity{
			Kind: "Service",
			Metadata: entity.EntityMetadata{
				Name:        in.Name,
				Namespace:   in.Namespace,
				Title:       in.Title,
				Description: in.Description,
				Owner:       in.Owner,
				Tags:        in.Tags,
			},
			Spec: map[string]any{},
		}
		setSpecString(e.Spec, "type", in.Type)
		setSpecString(e.Spec, "lifecycle", in.Lifecycle)
		setSpecString(e.Spec, "repoUrl", in.RepoURL)
		if len(e.Spec) == 0 {
			e.Spec = nil
		}
		e.SetDefaults()
		if claims := middleware.GetClaims(ctx); claims != nil {
			e.Metadata.CreatedBy = claims.Username
		}
		if err := h.Validator.Validate(&e); err != nil {
			return nil, nil, fmt.Errorf("validation failed: %w", err)
		}
		if !h.CanAccess(ctx, "entity", "Service", "write") {
			return nil, nil, errors.New("insufficient permissions to write Service entities")
		}
		if err := h.DB.CreateEntity(ctx, &e); err != nil {
			if errors.Is(err, entity.ErrEntityAlreadyExists) {
				return nil, nil, fmt.Errorf("service %s already exists in namespace %q", e.Metadata.Name, e.Metadata.Namespace)
			}
			return nil, nil, fmt.Errorf("create service: %w", err)
		}
		publishEntityEvent(h, events.EntityCreated, &e)
		writeMCPAuditEntry(ctx, h, middleware.GetClaims(ctx), "entity.created", &e, "", marshalEntityStateJSON(&e))
		return jsonResult(map[string]any{"entity": e, "created": true})
	})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "get_service",
		Description: "Return one Service entity by name. Use this before update_service or link_service when you need the current record.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in getServiceInput) (*mcpsdk.CallToolResult, any, error) {
		ns := namespaceOrDefault(in.Namespace)
		if !h.CanAccess(ctx, "entity", "Service", "read") {
			return nil, nil, errors.New("insufficient permissions to read Service entities")
		}
		e, err := h.DB.GetEntity(ctx, "Service", ns, in.Name)
		if err != nil {
			if errors.Is(err, entity.ErrEntityNotFound) {
				return nil, nil, fmt.Errorf("service %s not found in namespace %q", in.Name, ns)
			}
			return nil, nil, fmt.Errorf("get service: %w", err)
		}
		return jsonResult(e)
	})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "update_service",
		Description: "Partially update a Service. Only fields you set are changed; omitted fields stay as they are. Requires developer role or higher. Validates the result against the Service schema.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in updateServiceInput) (*mcpsdk.CallToolResult, any, error) {
		if err := requireDeveloperRole(ctx); err != nil {
			return nil, nil, err
		}
		return mutateService(ctx, h, in.Name, in.Namespace, func(e *entity.Entity) error {
			applyServiceUpdate(e, in)
			return nil
		})
	})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "link_service",
		Description: "Add or remove a relationship or URL on a Service without replacing the rest of the entity. target=relation uses relation plus kind and targetName (owner uses targetName as the team and sets metadata.owner). target=url uses title and url, with an optional icon. add is idempotent; remove does nothing when the link is already absent. Requires developer role or higher.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in linkServiceInput) (*mcpsdk.CallToolResult, any, error) {
		if err := requireDeveloperRole(ctx); err != nil {
			return nil, nil, err
		}
		return mutateService(ctx, h, in.Name, in.Namespace, func(e *entity.Entity) error {
			return applyServiceLink(e, in)
		})
	})
}

func mutateService(ctx context.Context, h *handlers.Handlers, name, namespace string, mutate func(*entity.Entity) error) (*mcpsdk.CallToolResult, any, error) {
	ns := namespaceOrDefault(namespace)
	if !h.CanAccess(ctx, "entity", "Service", "write") {
		return nil, nil, errors.New("insufficient permissions to write Service entities")
	}
	current, err := h.DB.GetEntity(ctx, "Service", ns, name)
	if err != nil {
		if errors.Is(err, entity.ErrEntityNotFound) {
			return nil, nil, fmt.Errorf("service %s not found in namespace %q", name, ns)
		}
		return nil, nil, fmt.Errorf("get service: %w", err)
	}
	before := marshalEntityStateJSON(current)
	if err := mutate(current); err != nil {
		return nil, nil, err
	}
	current.SetDefaults()
	if err := h.Validator.Validate(current); err != nil {
		return nil, nil, fmt.Errorf("validation failed: %w", err)
	}
	if err := h.DB.UpdateEntity(ctx, current); err != nil {
		return nil, nil, fmt.Errorf("update service: %w", err)
	}
	publishEntityEvent(h, events.EntityUpdated, current)
	writeMCPAuditEntry(ctx, h, middleware.GetClaims(ctx), "entity.updated", current, before, marshalEntityStateJSON(current))
	return jsonResult(map[string]any{"entity": current, "updated": true})
}

func applyServiceUpdate(e *entity.Entity, in updateServiceInput) {
	if in.Title != nil {
		e.Metadata.Title = *in.Title
	}
	if in.Description != nil {
		e.Metadata.Description = *in.Description
	}
	if in.Owner != nil {
		e.Metadata.Owner = *in.Owner
	}
	if in.Tags != nil {
		e.Metadata.Tags = *in.Tags
	}
	if e.Spec == nil {
		e.Spec = map[string]any{}
	}
	setOptionalSpecString(e.Spec, "type", in.Type)
	setOptionalSpecString(e.Spec, "lifecycle", in.Lifecycle)
	setOptionalSpecString(e.Spec, "repoUrl", in.RepoURL)
}

func applyServiceLink(e *entity.Entity, in linkServiceInput) error {
	op := strings.ToLower(strings.TrimSpace(in.Op))
	if op != "add" && op != "remove" {
		return errors.New("op must be add or remove")
	}
	if e.Spec == nil {
		e.Spec = map[string]any{}
	}
	switch strings.ToLower(strings.TrimSpace(in.Target)) {
	case "relation":
		return applyRelationLink(e, op, in)
	case "url":
		return applyURLLink(e, op, in)
	default:
		return errors.New("target must be relation or url")
	}
}

func applyRelationLink(e *entity.Entity, op string, in linkServiceInput) error {
	switch in.Relation {
	case "dependsOn", "deployedIn":
		if strings.TrimSpace(in.Kind) == "" || strings.TrimSpace(in.TargetName) == "" {
			return fmt.Errorf("%s requires kind and targetName", in.Relation)
		}
		refs := objectRefs(e.Spec, in.Relation)
		refs = mutateRefs(refs, op, in.Kind, in.TargetName)
		setObjectRefs(e.Spec, in.Relation, refs)
		return nil
	case "providesApi", "consumesApi":
		if in.Kind != "" && !strings.EqualFold(in.Kind, "API") {
			return fmt.Errorf("%s kind must be API", in.Relation)
		}
		if strings.TrimSpace(in.TargetName) == "" {
			return fmt.Errorf("%s requires targetName", in.Relation)
		}
		key := in.Relation + "s"
		names := stringItems(e.Spec, key)
		names = mutateNames(names, op, in.TargetName)
		setStringItems(e.Spec, key, names)
		return nil
	case "owner":
		if in.Kind != "" && !strings.EqualFold(in.Kind, "Team") {
			return errors.New("owner kind must be Team")
		}
		if strings.TrimSpace(in.TargetName) == "" {
			return errors.New("owner requires targetName")
		}
		if op == "add" {
			e.Metadata.Owner = in.TargetName
			return nil
		}
		if e.Metadata.Owner == in.TargetName {
			e.Metadata.Owner = ""
		}
		return nil
	default:
		return errors.New("relation must be dependsOn, providesApi, consumesApi, deployedIn, or owner")
	}
}

func applyURLLink(e *entity.Entity, op string, in linkServiceInput) error {
	if strings.TrimSpace(in.URL) == "" {
		return errors.New("url target requires url")
	}
	if op == "add" && strings.TrimSpace(in.Title) == "" {
		return errors.New("url target requires title when adding")
	}
	if in.Icon != "" && !linkIcons[in.Icon] {
		return fmt.Errorf("unsupported link icon %q", in.Icon)
	}
	links := objectRefs(e.Spec, "links")
	if op == "add" {
		for _, link := range links {
			if stringField(link, "url") == in.URL {
				return nil
			}
		}
		link := map[string]any{"title": in.Title, "url": in.URL}
		if in.Icon != "" {
			link["icon"] = in.Icon
		}
		links = append(links, link)
	} else {
		filtered := links[:0]
		for _, link := range links {
			if stringField(link, "url") == in.URL && (in.Title == "" || stringField(link, "title") == in.Title) {
				continue
			}
			filtered = append(filtered, link)
		}
		links = filtered
	}
	setObjectRefs(e.Spec, "links", links)
	return nil
}

func mutateRefs(refs []map[string]any, op, kind, name string) []map[string]any {
	if op == "add" {
		for _, ref := range refs {
			if stringField(ref, "kind") == kind && stringField(ref, "name") == name {
				return refs
			}
		}
		return append(refs, map[string]any{"kind": kind, "name": name})
	}
	filtered := refs[:0]
	for _, ref := range refs {
		if stringField(ref, "kind") == kind && stringField(ref, "name") == name {
			continue
		}
		filtered = append(filtered, ref)
	}
	return filtered
}

func mutateNames(names []string, op, name string) []string {
	if op == "add" {
		for _, existing := range names {
			if existing == name {
				return names
			}
		}
		return append(names, name)
	}
	filtered := names[:0]
	for _, existing := range names {
		if existing == name {
			continue
		}
		filtered = append(filtered, existing)
	}
	return filtered
}

func objectRefs(spec map[string]any, key string) []map[string]any {
	raw, ok := spec[key]
	if !ok || raw == nil {
		return nil
	}
	switch items := raw.(type) {
	case []map[string]any:
		return append([]map[string]any{}, items...)
	case []any:
		out := make([]map[string]any, 0, len(items))
		for _, item := range items {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			out = append(out, m)
		}
		return out
	default:
		return nil
	}
}

func setObjectRefs(spec map[string]any, key string, refs []map[string]any) {
	if len(refs) == 0 {
		delete(spec, key)
		return
	}
	items := make([]any, len(refs))
	for i, ref := range refs {
		items[i] = ref
	}
	spec[key] = items
}

func stringItems(spec map[string]any, key string) []string {
	raw, ok := spec[key]
	if !ok || raw == nil {
		return nil
	}
	switch items := raw.(type) {
	case []string:
		return append([]string{}, items...)
	case []any:
		out := make([]string, 0, len(items))
		for _, item := range items {
			s, ok := item.(string)
			if ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func setStringItems(spec map[string]any, key string, items []string) {
	if len(items) == 0 {
		delete(spec, key)
		return
	}
	// jsonschema validates decoded JSON, which uses []any rather than []string.
	raw := make([]any, len(items))
	for i, item := range items {
		raw[i] = item
	}
	spec[key] = raw
}

func stringField(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

func setSpecString(spec map[string]any, key, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	spec[key] = value
}

func setOptionalSpecString(spec map[string]any, key string, value *string) {
	if value == nil {
		return
	}
	if strings.TrimSpace(*value) == "" {
		delete(spec, key)
		return
	}
	spec[key] = *value
}

func namespaceOrDefault(namespace string) string {
	if namespace == "" {
		return entity.DefaultNamespace
	}
	return namespace
}

func publishEntityEvent(h *handlers.Handlers, eventType events.EventType, e *entity.Entity) {
	if h.Events == nil {
		return
	}
	h.Events.Publish(events.Event{
		Type: eventType,
		Data: map[string]any{
			"kind":      e.Kind,
			"name":      e.Metadata.Name,
			"namespace": e.Metadata.Namespace,
		},
	})
}
