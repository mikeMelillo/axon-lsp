package diag

import (
	"strings"
	"testing"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/index"
)

func TestTrioDiagnosticsOnlyAnalyzeSrc(t *testing.T) {
	m := testXetoManager(t, "+Funcs {\n inspect: Func { value: Obj? }\n}")
	const uri = "file:///records.trio"
	const source = `---
name: first
ruleOn: room and networkRef->dis=="EPMS" and dataHall
help:
    missingHeader()
    round(unknownHeader)
doc:
    absentHeader->dis
func
src:
    () => do
        local: 1
        round(local)
    end
ruleAfter: absentTail->dis
---
name: second
local: 1
func
src: () => inspect(local)`
	diagnostics := Validate(uri, source, m)
	if len(diagnostics) != 1 || diagnostics[0].Message != "Undefined variable: local" || diagnostics[0].Range.Start.Line != 19 || diagnostics[0].Range.Start.Character != len("src: () => inspect(") {
		t.Fatalf("metadata parsed as code, records leaked scope, or host mapping lost: %#v", diagnostics)
	}
	if refs := m.GetReferences("round"); len(refs) != 1 || refs[0].Range.Start.Line != 12 {
		t.Fatalf("references must include only src and retain host positions: %#v", refs)
	}
	// A document without src has no Axon, and deleting src clears old references.
	metadata := "name: metadataOnly\nruleOn: networkRef->dis\nhelp:\n    round(missingHeader)"
	if diagnostics := Validate(uri, metadata, m); len(diagnostics) != 0 || len(m.GetReferences("round")) != 0 {
		t.Fatalf("metadata-only document retained code diagnostics/references: %#v", diagnostics)
	}
}

func TestTrioDefcompMetadataIsNotAxon(t *testing.T) {
	m, err := index.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	const source = `---
name: aeFdcim_mechLoadVsDegreeDay
dis: "Mechanical Load vs Degree Days Per Room"
help:
    - Definition
    - Impact
    - Purpose
ruleOn: room and networkRef->dis=="EPMS" and dataHall
kpiRule
doc:
    Mechanical Load normalized by degree day from the local weather station.
func
overridable
src:
    defcomp
        target: {}
        mechLoad: {bind: "point and his and elec and power and roomRef=={{target->id}} and equipRef->mdp and (equipRef->main or equipRef->virtual)"}
        date: {}
        data: {readonly}
        out: {readonly}
        do

            if(mechLoad["siteRef"].isNull) throw "Load point has no site ref! Point: " + mechLoad.dis() + " for Room: " + target.dis()
            data = hisRead(mechLoad, date, {-limit}).energyNormByDegreeDay()

            out = {
                max: data.foldCol("v0", max),
                min: data.foldCol("v0", min),
                avg: data.foldCol("v0", avg)
            }

        end
    end`
	if diagnostics := Validate("file:///rule.trio", source, m); len(diagnostics) != 0 {
		t.Fatalf("valid defcomp record produced diagnostics: %#v", diagnostics)
	}
	broken := strings.Replace(source, "            out = {", "            absentBody->dis\n            out = {", 1)
	found := false
	for _, diagnostic := range Validate("file:///rule.trio", broken, m) {
		if diagnostic.Message == "Undefined variable: absentBody" {
			found = true
			if diagnostic.Range.Start.Line != 25 || diagnostic.Range.Start.Character != 12 {
				t.Fatalf("body diagnostic lost host position: %#v", diagnostic)
			}
		}
	}
	if !found {
		t.Fatal("a genuine undefined receiver inside defcomp src must still warn")
	}
}
