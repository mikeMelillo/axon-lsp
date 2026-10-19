package diag

import (
	"strings"
	"testing"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/index"
)

func TestTrioRequiredParameterWithDefaultedSiblings(t *testing.T) {
	m, err := index.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	const source = `name: sensorLocationToAisle
func
overridable
src:
    (location, rowNumLow: 3, rowNumHigh: 19) => do
        rowNum: location.toStr[3..4].parseNumber
        if(rowNumLow <= rowNum and rowNum <= rowNumHigh) return "coldAisle"
        else if(rowNum > rowNumHigh) return "hotAisle"
        else return "none"
    end`
	if diagnostics := Validate("file:///sensor.trio", source, m); len(diagnostics) != 0 {
		t.Fatalf("valid required/defaulted parameter mix warned: %#v", diagnostics)
	}
	broken := strings.Replace(source, "rowNum: location.toStr", "rowNum: missingLocation.toStr", 1)
	markerDefault := strings.Replace(source, "rowNumHigh: 19", "rowNumHigh: {-limit}", 1)
	if diagnostics := Validate("file:///sensor.trio", markerDefault, m); len(diagnostics) != 0 {
		t.Fatalf("removal-marker default lost required parameter bindings: %#v", diagnostics)
	}
	if diagnostics := Validate("file:///sensor.trio", broken, m); len(diagnostics) != 1 || diagnostics[0].Message != "Undefined variable: missingLocation" || diagnostics[0].Range.Start.Line != 5 {
		t.Fatalf("a genuine undefined name must still warn at its host position: %#v", diagnostics)
	}
}

func TestTrioInfeedRemovalDefault(t *testing.T) {
	m, err := index.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	const source = `name:stUtils_findTotalInfeedPower
func
doc:
    To build out later
src:
    (infeed, date, opts: {-limit}) => do
        thisPower: infeed.findPowerPoint()
        otherFilter: if(infeed.has("aSide")) "bSide" else "aSide"
        pdu: infeed->elecRef
        children: readAll(infeed and navName and infeedNum and parseFilter(otherFilter))
        otherPower: children.find(c=> c["infeedNum"] == infeed["infeedNum"]).findPowerPoint()
        data: hisRead([thisPower, otherPower], date, opts)
        .hisFoldCols(sum)
        .addMeta({
            inputInfeedPower: thisPower.dis(),
            siblingInfeedPower: otherPower.dis(),
            pdu: pdu.refDis()
        }).addColMeta("v0", {dis: "Infeed Power " + infeed->infeedNum})
        return data
    end`
	for _, d := range Validate("file:///infeed.trio", source, m) {
		if strings.Contains(d.Message, "variable: infeed") || strings.Contains(d.Message, "variable: limit") {
			t.Fatalf("marker default broke scope: %#v", d)
		}
	}
	broken := strings.Replace(source, "pdu: infeed->elecRef", "pdu: missingInfeed->elecRef", 1)
	found := false
	for _, d := range Validate("file:///infeed.trio", broken, m) {
		if d.Message == "Undefined variable: missingInfeed" {
			found = true
		}
	}
	if !found {
		t.Fatal("actual undefined body receiver must still warn")
	}
}
