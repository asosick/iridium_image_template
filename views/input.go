package views

import (
	"github.com/a-h/templ"
	"github.com/iridiumgo/iridium/core/blocks/bfield"
	"github.com/iridiumgo/iridium/core/collections"
	"github.com/iridiumgo/iridium/core/field"
	"github.com/iridiumgo/iridium/view/blocks/fields"
)

// inputBlock satisfies bfield.ITextInputBlock by combining Iridium's concrete
// field base (id/name/label/value) with its concrete text-input block
// (type/placeholder/icons/…). Rendering it through fields.NewInput gives the
// exact same markup and theme classes Iridium's own form inputs use.
type inputBlock struct {
	*field.BaseFieldConcrete
	*bfield.TextInputBlock
}

// fieldInput builds a themed Iridium text input for one template element and
// wires it to the shared Alpine `fields` scope via x-model, so typing updates
// the live preview overlay.
func fieldInput(el ElementData) templ.Component {
	base := field.NewBaseFieldConcrete(el.Field)
	base.LabelStr = el.Label
	base.AttributesMap = templ.Attributes{
		"x-model":      fieldExpr(el.Field),
		"autocomplete": "off",
	}

	block := &inputBlock{
		BaseFieldConcrete: base,
		TextInputBlock:    bfield.NewTextInputBlock(),
	}
	// The concrete input always renders a maxlength; the default is 100, which
	// is too tight for template copy. Use a generous cap so long promotional
	// text isn't silently truncated.
	block.MaxLengthInt = 2000

	return fields.NewInput(block).Component()
}

type nativeSelectBlock struct {
	*field.BaseFieldConcrete
	*bfield.SelectNativeBlock
}

func fontInput(el ElementData, options []FontData) templ.Component {
	base := field.NewBaseFieldConcrete(fontFieldName(el.Field))
	base.LabelStr = "Font"
	base.ValueArr = []string{el.Font}
	base.AttributesMap = templ.Attributes{
		"x-model": stylePropertyExpr(el.Field, "font"),
	}

	fontSelect := &nativeSelectBlock{
		BaseFieldConcrete: base,
		SelectNativeBlock: bfield.NewSelectNativeBlock(),
	}
	fontSelect.OptionsArr = collections.NewOrderedMap[string, string]()
	for _, option := range options {
		fontSelect.OptionsArr.Set(option.ID, option.Label)
	}
	return fields.NewSelectNative(fontSelect).Component()
}
