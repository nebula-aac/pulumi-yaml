// Copyright 2022-2025, Pulumi Corporation.  All rights reserved.

package ast

import (
	"fmt"
	"io"
	"reflect"
	"strings"
	"unicode"

	"github.com/hashicorp/hcl/v2"

	yamldiags "github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/diags"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/packages"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
)

type declNode struct {
	syntax syntax.Node
}

func decl(node syntax.Node) declNode {
	return declNode{node}
}

func (x *declNode) Syntax() syntax.Node {
	if x == nil {
		return nil
	}
	return x.syntax
}

type parseDecl interface {
	parse(name string, node syntax.Node) syntax.Diagnostics
}

type recordDecl interface {
	recordSyntax() *syntax.Node
}

type StringListDecl struct {
	declNode

	Elements []*StringExpr
}

type nonNilDecl interface {
	defaultValue() interface{}
}

func (d *StringListDecl) GetElements() []*StringExpr {
	if d == nil {
		return nil
	}
	return d.Elements
}

func (d *StringListDecl) parse(name string, node syntax.Node) syntax.Diagnostics {
	list, ok := node.(*syntax.ListNode)
	if !ok {
		return syntax.Diagnostics{syntax.NodeError(node, fmt.Sprintf("%v must be a list", name), "")}
	}

	var diags syntax.Diagnostics

	elements := make([]*StringExpr, list.Len())
	for i := range elements {
		ename := fmt.Sprintf("%s[%d]", name, i)
		ediags := parseField(ename, reflect.ValueOf(&elements[i]).Elem(), list.Index(i))
		diags.Extend(ediags...)
	}
	d.Elements = elements

	return diags
}

type ConfigMapEntry struct {
	syntax syntax.ObjectPropertyDef
	Key    *StringExpr
	Value  *ConfigParamDecl
}

type ConfigMapDecl struct {
	declNode

	Entries []ConfigMapEntry
}

func (d *ConfigMapDecl) defaultValue() interface{} {
	return &ConfigMapDecl{}
}

func (d *ConfigMapDecl) parse(name string, node syntax.Node) syntax.Diagnostics {
	obj, ok := node.(*syntax.ObjectNode)
	if !ok {
		return syntax.Diagnostics{syntax.NodeError(node, fmt.Sprintf("%v must be an object", name), "")}
	}

	var diags syntax.Diagnostics

	entries := make([]ConfigMapEntry, obj.Len())
	for i := range entries {
		kvp := obj.Index(i)
		if _, ok := kvp.Value.(*syntax.ObjectNode); !ok {
			valueExpr, vdiags := ParseExpr(kvp.Value)
			diags.Extend(vdiags...)
			entries[i] = ConfigMapEntry{
				syntax: kvp,
				Key:    StringSyntax(kvp.Key),
				Value: &ConfigParamDecl{
					Value: valueExpr,
				},
			}
		} else {
			var v *ConfigParamDecl
			vname := fmt.Sprintf("%s.%s", name, kvp.Key.Value())
			vdiags := parseField(vname, reflect.ValueOf(&v).Elem(), kvp.Value)
			diags.Extend(vdiags...)

			entries[i] = ConfigMapEntry{
				syntax: kvp,
				Key:    StringSyntax(kvp.Key),
				Value:  v,
			}
		}
	}
	d.Entries = entries

	return diags
}

type VariablesMapEntry struct {
	syntax syntax.ObjectPropertyDef
	Key    *StringExpr
	Value  Expr
}

type VariablesMapDecl struct {
	declNode

	Entries []VariablesMapEntry
}

func (d *VariablesMapDecl) defaultValue() interface{} {
	return &VariablesMapDecl{}
}

func (d *VariablesMapDecl) parse(name string, node syntax.Node) syntax.Diagnostics {
	obj, ok := node.(*syntax.ObjectNode)
	if !ok {
		return syntax.Diagnostics{syntax.NodeError(node, fmt.Sprintf("%v must be an object", name), "")}
	}

	var diags syntax.Diagnostics

	entries := make([]VariablesMapEntry, obj.Len())
	for i := range entries {
		kvp := obj.Index(i)

		v, vdiags := ParseExpr(kvp.Value)
		diags.Extend(vdiags...)

		entries[i] = VariablesMapEntry{
			syntax: kvp,
			Key:    StringSyntax(kvp.Key),
			Value:  v,
		}
	}
	d.Entries = entries

	return diags
}

type ResourcesMapEntry struct {
	syntax syntax.ObjectPropertyDef
	Key    *StringExpr
	Value  *ResourceDecl
}

type ResourcesMapDecl struct {
	declNode

	Entries []ResourcesMapEntry
}

func (d *ResourcesMapDecl) defaultValue() interface{} {
	return &ResourcesMapDecl{}
}

func (d *ResourcesMapDecl) parse(name string, node syntax.Node) syntax.Diagnostics {
	obj, ok := node.(*syntax.ObjectNode)
	if !ok {
		return syntax.Diagnostics{syntax.NodeError(node, fmt.Sprintf("%v must be an object", name), "")}
	}

	var diags syntax.Diagnostics

	entries := make([]ResourcesMapEntry, obj.Len())
	for i := range entries {
		kvp := obj.Index(i)

		var v *ResourceDecl
		vname := fmt.Sprintf("%s.%s", name, kvp.Key.Value())
		vdiags := parseField(vname, reflect.ValueOf(&v).Elem(), kvp.Value)
		diags.Extend(vdiags...)

		entries[i] = ResourcesMapEntry{
			syntax: kvp,
			Key:    StringSyntax(kvp.Key),
			Value:  v,
		}
	}
	d.Entries = entries

	return diags
}

type PropertyMapEntry struct {
	syntax syntax.ObjectPropertyDef
	Key    *StringExpr
	Value  Expr
}

func (p PropertyMapEntry) Object() ObjectProperty {
	return ObjectProperty{
		syntax: p.syntax,
		Key:    p.Key,
		Value:  p.Value,
	}
}

type PropertyMapDecl struct {
	declNode

	Entries []PropertyMapEntry
}

func (d *PropertyMapDecl) defaultValue() interface{} {
	return &PropertyMapDecl{}
}

func (d *PropertyMapDecl) parse(name string, node syntax.Node) syntax.Diagnostics {
	d.syntax = node

	obj, ok := node.(*syntax.ObjectNode)
	if !ok {
		return syntax.Diagnostics{syntax.NodeError(node, fmt.Sprintf("%v must be an object", name), "")}
	}

	var diags syntax.Diagnostics

	entries := make([]PropertyMapEntry, obj.Len())
	for i := range entries {
		kvp := obj.Index(i)

		var v Expr
		vname := fmt.Sprintf("%s.%s", name, kvp.Key.Value())
		vdiags := parseField(vname, reflect.ValueOf(&v).Elem(), kvp.Value)
		diags.Extend(vdiags...)

		entries[i] = PropertyMapEntry{
			syntax: kvp,
			Key:    StringSyntax(kvp.Key),
			Value:  v,
		}
	}
	d.Entries = entries

	return diags
}

type PropertyMapOrExprDecl struct {
	declNode

	Expr        Expr
	PropertyMap *PropertyMapDecl
}

func (d *PropertyMapOrExprDecl) defaultValue() interface{} {
	return &PropertyMapOrExprDecl{}
}

func (d *PropertyMapOrExprDecl) parse(name string, node syntax.Node) syntax.Diagnostics {
	d.syntax = node

	obj, ok := node.(*syntax.ObjectNode)
	if ok {
		var diags syntax.Diagnostics

		entries := make([]PropertyMapEntry, obj.Len())
		for i := range entries {
			kvp := obj.Index(i)

			var v Expr
			vname := fmt.Sprintf("%s.%s", name, kvp.Key.Value())
			vdiags := parseField(vname, reflect.ValueOf(&v).Elem(), kvp.Value)
			diags.Extend(vdiags...)

			entries[i] = PropertyMapEntry{
				syntax: kvp,
				Key:    StringSyntax(kvp.Key),
				Value:  v,
			}
		}
		d.PropertyMap = &PropertyMapDecl{}
		d.PropertyMap.Entries = entries

		return diags
	}

	expr, diags := ParseExpr(node)
	d.Expr = expr
	return diags
}

type ConfigParamDecl struct {
	declNode

	Type    *StringExpr
	Name    *StringExpr
	Secret  *BooleanExpr
	Default Expr
	Value   Expr
	Items   *ConfigParamDecl
}

func (d *ConfigParamDecl) recordSyntax() *syntax.Node {
	return &d.syntax
}

func ConfigParamSyntax(node *syntax.ObjectNode, typ *StringExpr, name *StringExpr,
	secret *BooleanExpr, defaultValue Expr,
) *ConfigParamDecl {
	return &ConfigParamDecl{
		declNode: decl(node),
		Type:     typ,
		Name:     name,
		Secret:   secret,
		Default:  defaultValue,
	}
}

func ConfigParam(typ *StringExpr, name *StringExpr, defaultValue Expr, secret *BooleanExpr) *ConfigParamDecl {
	return ConfigParamSyntax(nil, typ, name, secret, defaultValue)
}

type ResourceOptionsDecl struct {
	declNode

	AdditionalSecretOutputs *StringListDecl
	Aliases                 *StringListDecl
	CustomTimeouts          *CustomTimeoutsDecl
	DeleteBeforeReplace     *BooleanExpr
	DependsOn               Expr
	IgnoreChanges           *StringListDecl
	Import                  *StringExpr
	Parent                  Expr
	Protect                 Expr
	Provider                Expr
	Providers               Expr
	Version                 *StringExpr
	PluginDownloadURL       *StringExpr
	ReplaceOnChanges        *StringListDecl
	RetainOnDelete          *BooleanExpr
	DeletedWith             Expr
}

func (d *ResourceOptionsDecl) defaultValue() interface{} {
	return &ResourceOptionsDecl{}
}

func (d *ResourceOptionsDecl) recordSyntax() *syntax.Node {
	return &d.syntax
}

func ResourceOptionsSyntax(node *syntax.ObjectNode,
	additionalSecretOutputs, aliases *StringListDecl, customTimeouts *CustomTimeoutsDecl,
	deleteBeforeReplace *BooleanExpr, dependsOn Expr, ignoreChanges *StringListDecl, importID *StringExpr,
	parent Expr, protect Expr, provider, providers Expr, version *StringExpr,
	pluginDownloadURL *StringExpr, replaceOnChanges *StringListDecl,
	retainOnDelete *BooleanExpr, deletedWith Expr,
) ResourceOptionsDecl {
	return ResourceOptionsDecl{
		declNode:                decl(node),
		AdditionalSecretOutputs: additionalSecretOutputs,
		Aliases:                 aliases,
		CustomTimeouts:          customTimeouts,
		DeleteBeforeReplace:     deleteBeforeReplace,
		DependsOn:               dependsOn,
		IgnoreChanges:           ignoreChanges,
		Import:                  importID,
		Parent:                  parent,
		Protect:                 protect,
		Provider:                provider,
		Version:                 version,
		PluginDownloadURL:       pluginDownloadURL,
		ReplaceOnChanges:        replaceOnChanges,
		RetainOnDelete:          retainOnDelete,
		DeletedWith:             deletedWith,
	}
}

func ResourceOptions(additionalSecretOutputs, aliases *StringListDecl,
	customTimeouts *CustomTimeoutsDecl, deleteBeforeReplace *BooleanExpr,
	dependsOn Expr, ignoreChanges *StringListDecl, importID *StringExpr, parent Expr,
	protect Expr, provider, providers Expr, version *StringExpr, pluginDownloadURL *StringExpr,
	replaceOnChanges *StringListDecl, retainOnDelete *BooleanExpr, deletedWith Expr,
) ResourceOptionsDecl {
	return ResourceOptionsSyntax(nil, additionalSecretOutputs, aliases, customTimeouts,
		deleteBeforeReplace, dependsOn, ignoreChanges, importID, parent, protect, provider, providers,
		version, pluginDownloadURL, replaceOnChanges, retainOnDelete, deletedWith)
}

type InvokeOptionsDecl struct {
	declNode

	DependsOn         Expr
	Parent            Expr
	Provider          Expr
	Version           *StringExpr
	PluginDownloadURL *StringExpr
}

func (d *InvokeOptionsDecl) defaultValue() interface{} {
	return &InvokeOptionsDecl{}
}

func (d *InvokeOptionsDecl) recordSyntax() *syntax.Node {
	return &d.syntax
}

type GetResourceDecl struct {
	declNode
	// We need to call the field Id instead of ID because we want the derived user field to be id instead of iD
	Id    Expr //nolint:revive
	State PropertyMapDecl
}

func (d *GetResourceDecl) defaultValue() interface{} {
	return &GetResourceDecl{}
}

func (d *GetResourceDecl) recordSyntax() *syntax.Node {
	return &d.syntax
}

func GetResourceSyntax(node *syntax.ObjectNode, id *StringExpr, state PropertyMapDecl) GetResourceDecl {
	return GetResourceDecl{
		declNode: decl(node),
		Id:       id,
		State:    state,
	}
}

func GetResource(id *StringExpr, state PropertyMapDecl) GetResourceDecl {
	return GetResourceSyntax(nil, id, state)
}

type ResourceDecl struct {
	declNode

	Type            *StringExpr
	Name            *StringExpr
	DefaultProvider *BooleanExpr
	Properties      PropertyMapOrExprDecl
	Options         ResourceOptionsDecl
	Get             GetResourceDecl
}

func (d *ResourceDecl) recordSyntax() *syntax.Node {
	return &d.syntax
}

// The names of exported fields.
func (*ResourceDecl) Fields() []string {
	return []string{"type", "name", "defaultprovider", "properties", "options", "get"}
}

func ResourceSyntax(node *syntax.ObjectNode, typ *StringExpr, name *StringExpr, defaultProvider *BooleanExpr,
	properties PropertyMapOrExprDecl, options ResourceOptionsDecl, get GetResourceDecl,
) *ResourceDecl {
	return &ResourceDecl{
		declNode:        decl(node),
		Type:            typ,
		Name:            name,
		DefaultProvider: defaultProvider,
		Properties:      properties,
		Options:         options,
		Get:             get,
	}
}

func Resource(
	typ *StringExpr,
	name *StringExpr,
	defaultProvider *BooleanExpr,
	properties PropertyMapOrExprDecl,
	options ResourceOptionsDecl,
	get GetResourceDecl,
) *ResourceDecl {
	return ResourceSyntax(nil, typ, name, defaultProvider, properties, options, get)
}

type CustomTimeoutsDecl struct {
	declNode

	Create *StringExpr
	Update *StringExpr
	Delete *StringExpr
}

func (d *CustomTimeoutsDecl) recordSyntax() *syntax.Node {
	return &d.syntax
}

func CustomTimeoutsSyntax(node *syntax.ObjectNode, create, update, del *StringExpr) *CustomTimeoutsDecl {
	return &CustomTimeoutsDecl{
		declNode: declNode{syntax: node},
		Create:   create,
		Update:   update,
		Delete:   del,
	}
}

func CustomTimeouts(create, update, del *StringExpr) *CustomTimeoutsDecl {
	return CustomTimeoutsSyntax(nil, create, update, del)
}

type Template interface {
	GetName() *StringExpr
	GetDescription() *StringExpr
	GetConfig() ConfigMapDecl
	GetVariables() VariablesMapDecl
	GetResources() ResourcesMapDecl
	GetOutputs() PropertyMapDecl
	GetSdks() []packages.PackageDecl

	NewDiagnosticWriter(w io.Writer, width uint, color bool) hcl.DiagnosticWriter
}

// ComponentDecl represents a Pulumi YAML component.
type ComponentDecl struct {
	syntax syntax.ObjectPropertyDef
	Key    *StringExpr
	Value  *ComponentParamDecl
}

type ComponentParamDecl struct {
	declNode

	Name        *StringExpr
	Description *StringExpr
	Inputs      ConfigMapDecl
	Variables   VariablesMapDecl
	Resources   ResourcesMapDecl
	Outputs     PropertyMapDecl
	Template    *TemplateDecl
}

func (d *ComponentParamDecl) GetName() *StringExpr {
	if d == nil {
		return nil
	}
	return d.Name
}

func (d *ComponentParamDecl) GetDescription() *StringExpr {
	if d == nil {
		return nil
	}
	return d.Description
}

func (d *ComponentParamDecl) GetConfig() ConfigMapDecl {
	if d == nil {
		return ConfigMapDecl{}
	}
	return d.Inputs
}

func (d *ComponentParamDecl) GetVariables() VariablesMapDecl {
	if d == nil {
		return VariablesMapDecl{}
	}
	return d.Variables
}

func (d *ComponentParamDecl) GetResources() ResourcesMapDecl {
	if d == nil {
		return ResourcesMapDecl{}
	}
	return d.Resources
}

func (d *ComponentParamDecl) GetOutputs() PropertyMapDecl {
	if d == nil {
		return PropertyMapDecl{}
	}
	return d.Outputs
}

func (d *ComponentParamDecl) GetSdks() []packages.PackageDecl {
	if d == nil {
		return nil
	}
	return d.Template.Sdks
}

func (d *ComponentParamDecl) NewDiagnosticWriter(w io.Writer, width uint, color bool) hcl.DiagnosticWriter {
	return d.Template.NewDiagnosticWriter(w, width, color)
}

func (d *ComponentParamDecl) recordSyntax() *syntax.Node {
	return &d.syntax
}

type ComponentListDecl struct {
	declNode

	Entries []ComponentDecl
}

func (d *ComponentListDecl) defaultValue() interface{} {
	return &ComponentListDecl{}
}

func (d *ComponentListDecl) parse(name string, node syntax.Node) syntax.Diagnostics {
	obj, ok := node.(*syntax.ObjectNode)
	if !ok {
		return syntax.Diagnostics{syntax.NodeError(node, fmt.Sprintf("%v must be an object", name), "")}
	}

	var diags syntax.Diagnostics

	entries := make([]ComponentDecl, obj.Len())
	for i := range entries {
		kvp := obj.Index(i)
		var v *ComponentParamDecl
		logname := fmt.Sprintf("%s.%s", name, kvp.Key.Value())
		vdiags := parseField(logname, reflect.ValueOf(&v).Elem(), kvp.Value)
		diags.Extend(vdiags...)
		if diags.HasErrors() {
			return diags
		}

		v.Name = String(kvp.Key.Value())
		entries[i] = ComponentDecl{
			syntax: kvp,
			Key:    StringSyntax(kvp.Key),
			Value:  v,
		}
	}
	d.Entries = entries

	return diags
}

// A TemplateDecl represents a Pulumi YAML template.
type TemplateDecl struct {
	source []byte

	syntax syntax.Node

	Name          *StringExpr
	Namespace     *StringExpr
	Description   *StringExpr
	Configuration ConfigMapDecl
	Config        ConfigMapDecl
	Variables     VariablesMapDecl
	Resources     ResourcesMapDecl
	Outputs       PropertyMapDecl
	Sdks          []packages.PackageDecl
	Components    ComponentListDecl
}

func (d *TemplateDecl) GetName() *StringExpr {
	if d == nil {
		return nil
	}
	return d.Name
}

func (d *TemplateDecl) GetDescription() *StringExpr {
	if d == nil {
		return nil
	}
	return d.Description
}

func (d *TemplateDecl) GetConfig() ConfigMapDecl {
	if d == nil {
		return ConfigMapDecl{}
	}
	// TODO: merge config and configuration (?)
	return d.Configuration
}

func (d *TemplateDecl) GetVariables() VariablesMapDecl {
	if d == nil {
		return VariablesMapDecl{}
	}
	return d.Variables
}

func (d *TemplateDecl) GetResources() ResourcesMapDecl {
	if d == nil {
		return ResourcesMapDecl{}
	}
	return d.Resources
}

func (d *TemplateDecl) GetOutputs() PropertyMapDecl {
	if d == nil {
		return PropertyMapDecl{}
	}
	return d.Outputs
}

func (d *TemplateDecl) GetSdks() []packages.PackageDecl {
	if d == nil {
		return nil
	}
	return d.Sdks
}

func (d *TemplateDecl) Syntax() syntax.Node {
	if d == nil {
		return nil
	}
	return d.syntax
}

func (d *TemplateDecl) recordSyntax() *syntax.Node {
	return &d.syntax
}

// NewDiagnosticWriter returns a new hcl.DiagnosticWriter that can be used to print diagnostics associated with the
// template.
func (d *TemplateDecl) NewDiagnosticWriter(w io.Writer, width uint, color bool) hcl.DiagnosticWriter {
	fileMap := map[string]*hcl.File{}
	if d.source != nil {
		if s := d.syntax; s != nil {
			fileMap[s.Syntax().Range().Filename] = &hcl.File{Bytes: d.source}
		}
	}
	return newDiagnosticWriter(w, fileMap, width, color)
}

func (d *TemplateDecl) Merge(other *TemplateDecl) error {
	if other == nil {
		return nil
	}
	if d.Name == nil {
		d.Name = other.Name
	} else if other.Name != nil {
		return fmt.Errorf("cannot merge templates with different names")
	}
	if d.Description == nil {
		d.Description = other.Description
	} else if other.Description != nil {
		return fmt.Errorf("cannot merge templates with different descriptions")
	}
	if d.Namespace == nil {
		d.Namespace = other.Namespace
	} else if other.Namespace != nil {
		return fmt.Errorf("cannot merge templates with different namespaces")
	}
	d.Config.Entries = append(d.Config.Entries, other.Config.Entries...)
	d.Components.Entries = append(d.Components.Entries, other.Components.Entries...)
	return nil
}

func parseTypeSpec(configDecl *ConfigParamDecl) (schema.TypeSpec, error) {
	typeSpec := schema.TypeSpec{}
	if configDecl.Type == nil {
		return typeSpec, fmt.Errorf("missing type")
	}
	switch configDecl.Type.Value {
	case "string":
		typeSpec.Type = "string"
	case "integer":
		typeSpec.Type = "integer"
	case "boolean":
		typeSpec.Type = "boolean"
	case "array":
		if configDecl.Items == nil {
			return typeSpec, fmt.Errorf("missing items")
		}
		itemsTypeSpec, err := parseTypeSpec(configDecl.Items)
		if err != nil {
			return typeSpec, err
		}
		typeSpec.Type = "array"
		typeSpec.Items = &itemsTypeSpec
	default:
		return typeSpec, fmt.Errorf("unknown type: %s", configDecl.Type.Value)
	}
	return typeSpec, nil
}

func (d *TemplateDecl) GenerateSchema() (schema.PackageSpec, error) {
	description := ""
	if d.Description != nil {
		description = d.Description.Value
	}
	namespace := ""
	if d.Namespace != nil {
		namespace = d.Namespace.Value
	}
	schemaDef := schema.PackageSpec{
		Name:        d.Name.Value,
		Description: description,
		Version:     "0.0.0",
		Namespace:   namespace,
		Language: map[string]schema.RawMessage{
			"nodejs": schema.RawMessage(`{"respectSchemaVersion": true}`),
			"python": schema.RawMessage(`{"respectSchemaVersion": true}`),
			"cshap":  schema.RawMessage(`{"respectSchemaVersion": true}`),
			"java":   schema.RawMessage(`{"respectSchemaVersion": true}`),
			"go":     schema.RawMessage(`{"respectSchemaVersion": true}`),
		},
	}

	resourcesDef := make(map[string]schema.ResourceSpec)
	for _, component := range d.Components.Entries {
		componentType := d.Name.Value + ":index:" + component.Key.Value
		resourceDef := schema.ResourceSpec{
			ObjectTypeSpec: schema.ObjectTypeSpec{
				Properties: map[string]schema.PropertySpec{},
				Type:       componentType,
				Required:   []string{},
			},
			InputProperties: map[string]schema.PropertySpec{},
			IsComponent:     true,
		}
		if component.Value.Description != nil {
			resourceDef.Description = component.Value.Description.Value
		}

		for _, input := range component.Value.Inputs.Entries {
			k, v := input.Key.Value, input.Value
			typeSpec, err := parseTypeSpec(input.Value)
			if err != nil {
				return schema.PackageSpec{}, err
			}
			def := schemaDefaultValue(v.Default)

			resourceDef.InputProperties[k] = schema.PropertySpec{
				TypeSpec: typeSpec,
				Default:  def,
				DefaultInfo: &schema.DefaultSpec{
					Environment: []string{k},
				},
				Secret: v.Secret != nil && v.Secret.Value,
			}
			if def == nil {
				resourceDef.RequiredInputs = append(resourceDef.RequiredInputs, k)
			}
		}

		properties := map[string]schema.PropertySpec{}
		for _, output := range component.Value.Outputs.Entries {
			k := output.Key.Value

			// TODO: evaluate actual type. For the first cut we're just returning `Any` here.
			typeSpec := schema.TypeSpec{
				Ref: "pulumi.json#/Any",
			}

			properties[k] = schema.PropertySpec{
				TypeSpec: typeSpec,
			}
			resourceDef.Required = append(resourceDef.Required, k)
		}
		resourceDef.Properties = properties

		resourcesDef[componentType] = resourceDef
	}

	schemaDef.Resources = resourcesDef

	return schemaDef, nil
}

func schemaDefaultValue(e Expr) interface{} {
	switch e := e.(type) {
	case *StringExpr:
		return e.Value
	case *NumberExpr:
		return e.Value
	case *BooleanExpr:
		return e.Value
	case nil:
		return nil
	default:
		panic(fmt.Sprintf("Unknown default value: %s", e))
	}
}

func TemplateSyntax(node *syntax.ObjectNode, description *StringExpr, configuration ConfigMapDecl,
	variables VariablesMapDecl, resources ResourcesMapDecl, outputs PropertyMapDecl,
) *TemplateDecl {
	return &TemplateDecl{
		syntax:        node,
		Description:   description,
		Configuration: configuration,
		Variables:     variables,
		Resources:     resources,
		Outputs:       outputs,
	}
}

// ParseTemplate parses a template from the given syntax node. The source text is optional, and is only used to print
// diagnostics.
func ParseTemplate(source []byte, node syntax.Node) (*TemplateDecl, syntax.Diagnostics) {
	template := TemplateDecl{source: source}

	diags := parseRecord("template", &template, node, false)
	// Ensure that all components have a reference back to the template they belong to.
	for i := range template.Components.Entries {
		template.Components.Entries[i].Value.Template = &template
	}
	return &template, diags
}

var (
	parseDeclType  = reflect.TypeOf((*parseDecl)(nil)).Elem()
	nonNilDeclType = reflect.TypeOf((*nonNilDecl)(nil)).Elem()
	recordDeclType = reflect.TypeOf((*recordDecl)(nil)).Elem()
	exprType       = reflect.TypeOf((*Expr)(nil)).Elem()
)

func parseField(name string, dest reflect.Value, node syntax.Node) syntax.Diagnostics {
	if node == nil {
		return nil
	}

	var v reflect.Value
	var diags syntax.Diagnostics

	if dest.CanAddr() && dest.Addr().Type().AssignableTo(nonNilDeclType) {
		// destination is T, and must be a record type (right now)
		defaultValue := (dest.Addr().Interface().(nonNilDecl)).defaultValue()
		switch x := defaultValue.(type) {
		case parseDecl:
			pdiags := x.parse(name, node)
			diags.Extend(pdiags...)
			v = reflect.ValueOf(defaultValue).Elem().Convert(dest.Type())
		case recordDecl:
			pdiags := parseRecord(name, x, node, true)
			diags.Extend(pdiags...)
			v = reflect.ValueOf(defaultValue).Elem().Convert(dest.Type())
		}
		dest.Set(v)
		return diags
	}

	switch {
	case dest.Type().AssignableTo(parseDeclType):
		// assume that dest is *T
		v = reflect.New(dest.Type().Elem())
		pdiags := v.Interface().(parseDecl).parse(name, node)
		diags.Extend(pdiags...)
	case dest.Type().AssignableTo(recordDeclType):
		// assume that dest is *T
		v = reflect.New(dest.Type().Elem())
		rdiags := parseRecord(name, v.Interface().(recordDecl), node, true)
		diags.Extend(rdiags...)
	case dest.Type().AssignableTo(exprType):
		x, xdiags := ParseExpr(node)
		diags.Extend(xdiags...)
		if diags.HasErrors() {
			return diags
		}

		xv := reflect.ValueOf(x)
		if !xv.Type().AssignableTo(dest.Type()) {
			diags.Extend(exprFieldTypeMismatchError(name, dest.Interface(), x))
		} else {
			v = xv
		}
	default:
		panic(fmt.Errorf("unexpected field of type %T", dest.Interface()))
	}

	if !diags.HasErrors() {
		dest.Set(v)
	}
	return diags
}

func parseRecord(objName string, dest recordDecl, node syntax.Node, noMatchWarning bool) syntax.Diagnostics {
	obj, ok := node.(*syntax.ObjectNode)
	if !ok {
		return syntax.Diagnostics{syntax.NodeError(node, fmt.Sprintf("%v must be an object", objName), "")}
	}
	*dest.recordSyntax() = obj
	contract.Assertf(*dest.recordSyntax() == obj, "%s.recordSyntax took by value, so the assignment failed", objName)

	v := reflect.ValueOf(dest).Elem()
	t := v.Type()

	var diags syntax.Diagnostics
	for i := 0; i < obj.Len(); i++ {
		kvp := obj.Index(i)

		key := kvp.Key.Value()
		var hasMatch bool
		for _, f := range reflect.VisibleFields(t) {
			if f.IsExported() && strings.EqualFold(f.Name, key) {
				diags.Extend(syntax.UnexpectedCasing(kvp.Key.Syntax().Range(), camel(f.Name), key))
				diags.Extend(parseField(camel(f.Name), v.FieldByIndex(f.Index), kvp.Value)...)
				hasMatch = true
				break
			}
		}

		if !hasMatch && noMatchWarning {
			var fieldNames []string
			for i := 0; i < t.NumField(); i++ {
				f := t.Field(i)
				if f.IsExported() {
					fieldNames = append(fieldNames, fmt.Sprintf("'%s'", camel(f.Name)))
				}
			}
			formatter := yamldiags.NonExistentFieldFormatter{
				ParentLabel: fmt.Sprintf("Object '%s'", objName),
				Fields:      fieldNames,
			}
			msg, detail := formatter.MessageWithDetail(key, fmt.Sprintf("Field '%s'", key))
			nodeError := syntax.NodeError(kvp.Key, msg, detail)
			nodeError.Severity = hcl.DiagWarning
			diags = append(diags, nodeError)
		}

	}

	return diags
}

func exprFieldTypeMismatchError(name string, expected interface{}, actual Expr) *syntax.Diagnostic {
	var typeName string
	switch expected.(type) {
	case *NullExpr:
		typeName = "null"
	case *BooleanExpr:
		typeName = "a boolean value"
	case *NumberExpr:
		typeName = "a number"
	case *StringExpr:
		typeName = "a string"
	case *SymbolExpr:
		typeName = "a symbol"
	case *InterpolateExpr:
		typeName = "an interpolated string"
	case *ListExpr:
		typeName = "a list"
	case *ObjectExpr:
		typeName = "an object"
	case BuiltinExpr:
		typeName = "a builtin function call"
	default:
		typeName = fmt.Sprintf("a %T", expected)
	}
	return ExprError(actual, fmt.Sprintf("%v must be %v", name, typeName), "")
}

func camel(s string) string {
	if s == "" {
		return ""
	}
	name := []rune(s)
	name[0] = unicode.ToLower(name[0])
	return string(name)
}
