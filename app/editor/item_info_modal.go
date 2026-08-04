package editor

// Item-info popup (2026-08-01 user request): right-click a sellable cell on
// either grid to see its full detail -- icon, category/subcategory, in-game
// description, and (for weapons/armor/spells) scaling/requirements/negation/
// FP cost. Mirrors pending_edits.go's modal shape (title+X, gold rule,
// scrollable body over widgets.Backdrop) and its Press-based scrim-close
// fix from Phase 1 (a panel-scoped press tag alongside the whole-modal one,
// so a click on the scrim closes on the very first press).

import (
	"fmt"
	"image"
	"image/color"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"er_merchant_editor/app/catalog"
	"er_merchant_editor/app/editor/widgets"
)

// itemInfoScrim matches pending_edits.go's own scrim color -- same "dim
// everything behind a blocking dialog" treatment.
var itemInfoScrimColor = pendingModalScrim

// itemInfoIconSize is the popup's own icon size -- bigger than any grid
// cell's default so the item reads clearly as the dialog's subject (it's
// centered at the top of the card, ER-menu style).
const itemInfoIconSize = unit.Dp(96)

// openItemInfo shows the popup for item id at upgrade level `level` (0 for a
// non-weapon or a +0 weapon). Either grid's right-click handler: the catalog
// passes the clamped PickLevel, a merchant row its own reinforcement level, so
// the weapon card's attack/scaling reflect the "+N" actually in play.
// Re-opening on the same id while already open is a no-op; right-clicking a
// DIFFERENT item while open just retargets it.
func (s *State) openItemInfo(id int64, level int) {
	s.itemInfoID = id
	s.itemInfoLevel = level
	s.itemInfoOpen = true
}

func (s *State) closeItemInfo() {
	s.itemInfoOpen = false
}

// layoutItemInfoOverlay draws the popup: a full-window scrim plus a
// centered bordered panel, same mechanics as layoutPendingOverlay.
func (s *State) layoutItemInfoOverlay(gtx layout.Context, th *material.Theme) {
	if !s.itemInfoOpen {
		return
	}
	if s.itemInfoCloseBtn.Clicked(gtx) {
		s.closeItemInfo()
	}
	for s.itemInfoScrim.Clicked(gtx) {
		s.closeItemInfo()
	}

	widgets.Backdrop(gtx,
		widgets.BackdropStyle{
			Scrim:        itemInfoScrimColor,
			PanelBg:      colorPanelBg,
			BorderColor:  colorDivider,
			BorderWidth:  unit.Dp(1),
			CornerRadius: unit.Dp(4),
			Inset:        unit.Dp(16),
		},
		&s.itemInfoScrim,
		&s.itemInfoPanelBlocker,
		&s.itemInfoScrimPressTag,
		s.closeItemInfo,
		func(gtx *layout.Context) {
			maxW := gtx.Dp(unit.Dp(440))
			if avail := gtx.Constraints.Max.X - gtx.Dp(unit.Dp(48)); avail < maxW {
				maxW = avail
			}
			gtx.Constraints.Max.X = maxW
			gtx.Constraints.Max.Y = gtx.Constraints.Max.Y * 7 / 10
		},
		func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return s.layoutItemInfoPanel(gtx, th)
		},
		func(gtx layout.Context) {
			s.pressListener(gtx, &s.itemInfoModalTag, gtx.Constraints.Max, &s.itemInfoModalHit)
			s.layoutFooterStatusOverlay(gtx, th)
		},
	)
}

func (s *State) layoutItemInfoPanel(gtx layout.Context, th *material.Theme) layout.Dimensions {
	item := s.itemByID(s.itemInfoID)
	if item == nil {
		s.closeItemInfo()
		return layout.Dimensions{}
	}
	detail, _ := s.Catalog.ItemDetails(s.itemInfoID)

	// Weapons show stats at the "+N" the popup was opened at (catalog PickLevel
	// or a merchant row's own level); the scaled copy never mutates the cached
	// base detail. Ammunition shares the weapon table but isn't reinforced.
	name := strings.ToUpper(item.Name)
	if detail.Weapon != nil && s.itemInfoLevel > 0 && item.Category != "arrows_and_bolts" {
		scaled := s.Catalog.ReinforcedWeapon(s.itemInfoID, s.itemInfoLevel, *detail.Weapon)
		detail.Weapon = &scaled
		name = fmt.Sprintf("%s +%d", name, s.itemInfoLevel)
	}

	title := material.H6(th, name)
	header := layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, title.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return formButton(th, &s.itemInfoCloseBtn, "X").Layout(gtx)
		}),
	)

	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return header }),
		layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
		layout.Rigid(goldRule),
		layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutItemInfoBody(gtx, th, item, detail)
		}),
	)
	return dims
}

// layoutItemInfoBody draws the ER-menu-styled detail card: a centered icon,
// the category line, then two-column stat panels (Attack|Guard, Scaling|
// Requires for weapons; Negation|Resistance for armor; Requires|Cost for
// spells; Effect|Scaling for plain consumables), and finally the flavor
// description. Kept inside the scrollable itemInfoList so tall cards scroll.
func (s *State) layoutItemInfoBody(gtx layout.Context, th *material.Theme, item *catalog.Item, detail catalog.ItemDetail) layout.Dimensions {
	var rows []layout.Widget
	add := func(w layout.Widget) { rows = append(rows, w) }
	gap := func(dp int) { add(layout.Spacer{Height: unit.Dp(dp)}.Layout) }

	// Centered icon.
	add(func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceSides}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return framedIcon(gtx, s.Icons.Get(item.IconPath), itemInfoIconSize)
			}),
		)
	})
	// Centered category / subcategory.
	cat := categoryLabels[item.Category]
	if cat == "" {
		cat = item.Category
	}
	if item.SubCategory != "" {
		cat = cat + " / " + item.SubCategory
	}
	gap(8)
	add(func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceSides}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Body2(th, cat)
				lbl.Color = colorMuted
				return lbl.Layout(gtx)
			}),
		)
	})
	gap(12)

	// Case-by-case card body. No trailing flavor paragraph -- stats/effect
	// panels are the whole content, so the card stays short and scroll-free
	// (user request). Ammunition carries a weapon block but only meaningful
	// attack values, so it gets its own attack-only panel rather than the
	// full guard/scaling/requires weapon card.
	switch {
	case item.Category == "arrows_and_bolts" && detail.Weapon != nil:
		s.appendAmmoPanel(&rows, th, detail.Weapon)
	case detail.Weapon != nil:
		s.appendWeaponPanels(&rows, th, detail.Weapon, detail.Weight)
	case detail.Armor != nil:
		s.appendArmorPanels(&rows, th, detail.Armor, detail.Weight)
	case detail.Spell != nil:
		s.appendSpellPanels(&rows, th, detail)
	default:
		s.appendDescriptionOnly(&rows, th, item.Category, detail)
	}

	s.itemInfoList.Axis = layout.Vertical
	return material.List(th, &s.itemInfoList).Layout(gtx, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
		return rows[i](gtx)
	})
}

// statRow is one "Label   Value" line inside a statPanel. labelColor zero
// falls back to the muted secondary tone.
type statRow struct {
	label      string
	value      string
	labelColor color.NRGBA
}

func (s *State) appendWeaponPanels(rows *[]layout.Widget, th *material.Theme, w *catalog.WeaponDetail, weight float64) {
	attack := []statRow{
		{"Phys", itoa(w.PhysDamage), color.NRGBA{}},
		{"Mag", itoa(w.MagDamage), elemMag},
		{"Fire", itoa(w.FireDamage), elemFire},
		{"Ligt", itoa(w.LitDamage), elemLightning},
		{"Holy", itoa(w.HolyDamage), elemHoly},
		{"Crit", itoa(w.Crit), colorError},
	}
	guard := []statRow{
		{"Phys", itoa(w.GuardPhys), color.NRGBA{}},
		{"Mag", itoa(w.GuardMag), elemMag},
		{"Fire", itoa(w.GuardFire), elemFire},
		{"Ligt", itoa(w.GuardLit), elemLightning},
		{"Holy", itoa(w.GuardHoly), elemHoly},
		{"Boost", itoa(w.GuardBoost), color.NRGBA{}},
	}
	scaling := gradeRows([]statRow{
		{"Str", scalingGrade(w.ScaleStr), color.NRGBA{}},
		{"Dex", scalingGrade(w.ScaleDex), color.NRGBA{}},
		{"Int", scalingGrade(w.ScaleInt), color.NRGBA{}},
		{"Fai", scalingGrade(w.ScaleFai), color.NRGBA{}},
		{"Arc", scalingGrade(w.ScaleArc), color.NRGBA{}},
	})
	requires := nonZeroRows([]struct {
		label string
		val   uint32
	}{
		{"Str", w.ReqStr}, {"Dex", w.ReqDex}, {"Int", w.ReqInt}, {"Fai", w.ReqFai}, {"Arc", w.ReqArc},
	})
	if len(scaling) == 0 {
		scaling = []statRow{{"—", "", color.NRGBA{}}}
	}
	if len(requires) == 0 {
		requires = []statRow{{"—", "", color.NRGBA{}}}
	}

	*rows = append(*rows,
		s.panelPairWidget(th, "Attack", attack, "Guard", guard),
		layout.Spacer{Height: unit.Dp(8)}.Layout,
		s.panelPairWidget(th, "Scaling", scaling, "Requires", requires),
	)
	if p := passiveRows(w.Passives); len(p) > 0 {
		*rows = append(*rows,
			layout.Spacer{Height: unit.Dp(8)}.Layout,
			s.singlePanelWidget(th, "Passive", p),
		)
	}
	*rows = append(*rows, layout.Spacer{Height: unit.Dp(8)}.Layout, weightLine(th, weight))
}

func (s *State) appendArmorPanels(rows *[]layout.Widget, th *material.Theme, a *catalog.ArmorDetail, weight float64) {
	negation := []statRow{
		{"Phys", pct(a.Physical), color.NRGBA{}},
		{"Strike", pct(a.Strike), color.NRGBA{}},
		{"Slash", pct(a.Slash), color.NRGBA{}},
		{"Pierce", pct(a.Pierce), color.NRGBA{}},
		{"Mag", pct(a.Magic), elemMag},
		{"Fire", pct(a.Fire), elemFire},
		{"Ligt", pct(a.Lightning), elemLightning},
		{"Holy", pct(a.Holy), elemHoly},
	}
	resist := []statRow{
		{"Immunity", itoa(a.Immunity), color.NRGBA{}},
		{"Robustness", itoa(a.Robustness), color.NRGBA{}},
		{"Focus", itoa(a.Focus), color.NRGBA{}},
		{"Vitality", itoa(a.Vitality), color.NRGBA{}},
		{"Poise", trimFloat(a.Poise), color.NRGBA{}},
	}
	*rows = append(*rows,
		s.panelPairWidget(th, "Negation", negation, "Resistance", resist),
		layout.Spacer{Height: unit.Dp(8)}.Layout,
		weightLine(th, weight),
	)
}

func (s *State) appendSpellPanels(rows *[]layout.Widget, th *material.Theme, d catalog.ItemDetail) {
	sp := d.Spell
	requires := nonZeroRows([]struct {
		label string
		val   uint32
	}{
		{"Int", sp.ReqInt}, {"Fai", sp.ReqFai}, {"Arc", sp.ReqArc},
	})
	if len(requires) == 0 {
		requires = []statRow{{"—", "", color.NRGBA{}}}
	}
	cost := []statRow{
		{"FP Cost", itoa(sp.FPCost), color.NRGBA{}},
		{"Slots", itoa(sp.Slots), color.NRGBA{}},
	}
	if d.Description != "" {
		*rows = append(*rows, s.singlePanelWidget(th, "Effect", []statRow{{d.Description, "", color.NRGBA{}}}), layout.Spacer{Height: unit.Dp(8)}.Layout)
	}
	*rows = append(*rows, s.panelPairWidget(th, "Requires", requires, "Cost", cost))
}

// appendAmmoPanel renders arrows/bolts: a single Attack panel of the non-zero
// damage types (+ any status passive). Guard/scaling/requires/crit don't
// apply to ammunition even though it shares the weapon param table.
func (s *State) appendAmmoPanel(rows *[]layout.Widget, th *material.Theme, w *catalog.WeaponDetail) {
	attack := nonZeroRowsColored([]coloredVal{
		{"Phys", w.PhysDamage, color.NRGBA{}},
		{"Mag", w.MagDamage, elemMag},
		{"Fire", w.FireDamage, elemFire},
		{"Ligt", w.LitDamage, elemLightning},
		{"Holy", w.HolyDamage, elemHoly},
	})
	if len(attack) == 0 {
		attack = []statRow{{"—", "", color.NRGBA{}}}
	}
	if p := passiveRows(w.Passives); len(p) > 0 {
		*rows = append(*rows, s.panelPairWidget(th, "Attack", attack, "Passive", p))
		return
	}
	*rows = append(*rows, s.singlePanelWidget(th, "Attack", attack))
}

// appendDescriptionOnly renders items whose only content is their text
// (consumables/tools, talismans, spirit ashes, key items, info notes, ...):
// one box holding the effect/description, plus a weight line when the item
// has weight. Damage throwables additionally get a Scaling panel beside the
// Effect box (graded from their virtual-weapon coefficients). No empty
// Scaling/FP placeholders otherwise (user request).
func (s *State) appendDescriptionOnly(rows *[]layout.Widget, th *material.Theme, category string, d catalog.ItemDetail) {
	text := d.Description
	if text == "" {
		text = "No additional details."
	}
	effectRow := []statRow{{text, "", color.NRGBA{}}}
	if sd := d.Scaling; sd != nil {
		scaling := gradeRows([]statRow{
			{"Str", scalingGrade(sd.Str), color.NRGBA{}},
			{"Dex", scalingGrade(sd.Dex), color.NRGBA{}},
			{"Int", scalingGrade(sd.Int), color.NRGBA{}},
			{"Fai", scalingGrade(sd.Fai), color.NRGBA{}},
			{"Arc", scalingGrade(sd.Arc), color.NRGBA{}},
		})
		*rows = append(*rows, s.panelPairWidget(th, descBoxTitle(category), effectRow, "Scaling", scaling))
	} else {
		*rows = append(*rows, s.singlePanelWidget(th, descBoxTitle(category), effectRow))
	}
	if d.Weight > 0 {
		*rows = append(*rows, layout.Spacer{Height: unit.Dp(8)}.Layout, weightLine(th, d.Weight))
	}
}

// descBoxTitle names the single text box: "Effect" for items whose text is an
// action (consumables/tools/talismans), "Description" otherwise.
func descBoxTitle(category string) string {
	switch category {
	case "tools", "talismans":
		return "Effect"
	default:
		return "Description"
	}
}

func weightLine(th *material.Theme, weight float64) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Body2(th, "Weight")
				lbl.Color = colorMuted
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(material.Body2(th, trimFloat(weight)).Layout),
		)
	}
}

// panelPairWidget lays two statPanels side by side (equal width).
func (s *State) panelPairWidget(th *material.Theme, ltitle string, lrows []statRow, rtitle string, rrows []statRow) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return s.statPanel(gtx, th, ltitle, lrows)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return s.statPanel(gtx, th, rtitle, rrows)
			}),
		)
	}
}

func (s *State) singlePanelWidget(th *material.Theme, title string, rows []statRow) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return s.statPanel(gtx, th, title, rows)
	}
}

// statPanel draws a titled bordered card: a gold section title, a thin rule,
// then label/value rows. It expands to the width it's given so a pair aligns.
func (s *State) statPanel(gtx layout.Context, th *material.Theme, title string, rows []statRow) layout.Dimensions {
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	return infoCardBox(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		children := []layout.FlexChild{
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				t := material.Body2(th, title)
				t.Color = colorAmber
				t.Font.Weight = 600
				return t.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(3)}.Layout),
			layout.Rigid(statPanelRule),
			layout.Rigid(layout.Spacer{Height: unit.Dp(5)}.Layout),
		}
		for _, r := range rows {
			r := r
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return statRowWidget(gtx, th, r)
			}))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

// statRowWidget draws one "Label ....... Value" line; a value-less row (used
// for wrapped effect/description text) just lays the label full width.
func statRowWidget(gtx layout.Context, th *material.Theme, r statRow) layout.Dimensions {
	lbl := material.Body2(th, r.label)
	if r.value == "" {
		lbl.Color = colorFg
		return lbl.Layout(gtx)
	}
	lbl.Color = colorMuted
	if r.labelColor != (color.NRGBA{}) {
		lbl.Color = r.labelColor
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, lbl.Layout),
		layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
		layout.Rigid(material.Body2(th, r.value).Layout),
	)
}

// statPanelRule is the thin accent line under a stat panel's title. Unlike
// the modal header's bold 2dp goldRule, this is a hairline (1dp) amber at
// reduced alpha so it reads as a delicate underline inside the small boxes
// rather than a heavy gold bar (user feedback: the solid rule looked ugly in
// the Effect/Scaling cards).
func statPanelRule(gtx layout.Context) layout.Dimensions {
	h := gtx.Dp(unit.Dp(1))
	sz := image.Pt(gtx.Constraints.Max.X, h)
	c := colorAmber
	c.A = 0x70
	paint.FillShape(gtx.Ops, c, clip.Rect{Max: sz}.Op())
	return layout.Dimensions{Size: sz}
}

// infoCardBox fills w's content with the input-panel background and a 1dp
// divider border -- the "stat panel" chrome, distinct from groupBox (which
// is border-only, for the docked edit bar).
func infoCardBox(gtx layout.Context, w layout.Widget) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := layout.UniformInset(unit.Dp(8)).Layout(gtx, w)
	call := macro.Stop()

	paint.FillShape(gtx.Ops, colorInputBg, clip.UniformRRect(image.Rectangle{Max: dims.Size}, gtx.Dp(unit.Dp(4))).Op(gtx.Ops))
	call.Add(gtx.Ops)
	widget.Border{Color: colorDivider, Width: unit.Dp(1), CornerRadius: unit.Dp(4)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions { return dims })
	return dims
}

// scalingGrade maps a raw correctX coefficient to Elden Ring's letter grade
// (the widely-used threshold approximation of CalcCorrectGraph: S>=175,
// A>=140, B>=90, C>=60, D>=25, else E; 0 -> none). Verified against the
// wiki for e.g. Dagger (Str 35 -> D, Dex 65 -> C).
func scalingGrade(raw uint32) string {
	switch {
	case raw == 0:
		return ""
	case raw >= 175:
		return "S"
	case raw >= 140:
		return "A"
	case raw >= 90:
		return "B"
	case raw >= 60:
		return "C"
	case raw >= 25:
		return "D"
	default:
		return "E"
	}
}

// gradeRows keeps only rows whose scaling grade is non-empty.
func gradeRows(in []statRow) []statRow {
	var out []statRow
	for _, r := range in {
		if r.value != "" {
			out = append(out, r)
		}
	}
	return out
}

// nonZeroRows renders label/number rows, dropping zero-valued entries.
func nonZeroRows(in []struct {
	label string
	val   uint32
}) []statRow {
	var out []statRow
	for _, r := range in {
		if r.val != 0 {
			out = append(out, statRow{r.label, itoa(r.val), color.NRGBA{}})
		}
	}
	return out
}

// coloredVal is a label/number/tint triple for nonZeroRowsColored.
type coloredVal struct {
	label string
	val   uint32
	color color.NRGBA
}

// nonZeroRowsColored is nonZeroRows with per-row label tints (for ammo attack
// element colors), dropping zero-valued entries.
func nonZeroRowsColored(in []coloredVal) []statRow {
	var out []statRow
	for _, r := range in {
		if r.val != 0 {
			out = append(out, statRow{r.label, itoa(r.val), r.color})
		}
	}
	return out
}

// passiveRows turns a weapon's named status effects into "Blood Loss  50"
// rows (the value is the buildup; some resident effects have none).
func passiveRows(ps []catalog.WeaponPassive) []statRow {
	var out []statRow
	for _, p := range ps {
		v := ""
		if p.Value != 0 {
			v = fmt.Sprintf("%d", p.Value)
		}
		out = append(out, statRow{p.Label, v, color.NRGBA{}})
	}
	return out
}

func itoa(v uint32) string { return fmt.Sprintf("%d", v) }

func pct(v float64) string { return fmt.Sprintf("%.1f%%", v) }

func trimFloat(v float64) string { return strings.TrimSuffix(fmt.Sprintf("%.1f", v), ".0") }

// Elemental label tints -- fixed semantic colors (not theme-derived) chosen
// to stay legible on both the light and dark input-panel backgrounds.
var (
	elemMag       = color.NRGBA{R: 0x5A, G: 0x9A, B: 0xD8, A: 0xFF}
	elemFire      = color.NRGBA{R: 0xD8, G: 0x6A, B: 0x38, A: 0xFF}
	elemLightning = color.NRGBA{R: 0xC8, G: 0xA0, B: 0x30, A: 0xFF}
	elemHoly      = color.NRGBA{R: 0xC2, G: 0x9A, B: 0x48, A: 0xFF}
)
