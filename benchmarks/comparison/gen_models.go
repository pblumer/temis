//go:build ignore

// gen_models.go writes the shared DMN 1.3 benchmark models used by BOTH the
// Temis and the Drools harness, so the comparison evaluates byte-for-byte
// identical inputs. Run: go run gen_models.go
package main

import (
	"fmt"
	"os"
	"strings"
)

// DMN 1.3 namespace — the widest common ground: Drools supports it fully and
// Temis reads it (tolerant 1.3/1.4/1.5). Using one namespace guarantees both
// engines parse the same document rather than dialect-specific variants.
const ns = "http://www.omg.org/spec/DMN/20191111/MODEL/"

func header(name string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>`+"\n"+
		`<definitions xmlns="%s" namespace="http://temis.example/bench/%s" name="%s" id="def_%s">`+"\n", ns, name, name, name)
}

// stringTable: two string inputs matched by equality, n rules, UNIQUE — the most
// common real-world DMN table shape.
func stringTable(n int) string {
	seasons := []string{"Winter", "Spring", "Summer", "Autumn"}
	var b strings.Builder
	b.WriteString(header("StringTable"))
	for _, name := range []string{"Season", "Region"} {
		fmt.Fprintf(&b, `  <inputData id="i_%s" name="%s"><variable name="%s" typeRef="string"/></inputData>`+"\n", name, name, name)
	}
	b.WriteString(`  <decision id="d_Menu" name="Menu">` + "\n")
	b.WriteString(`    <variable name="Menu" typeRef="string"/>` + "\n")
	for _, name := range []string{"Season", "Region"} {
		fmt.Fprintf(&b, `    <informationRequirement id="ir_%s"><requiredInput href="#i_%s"/></informationRequirement>`+"\n", name, name)
	}
	b.WriteString(`    <decisionTable id="dt_Menu" hitPolicy="UNIQUE">` + "\n")
	for _, name := range []string{"Season", "Region"} {
		fmt.Fprintf(&b, `      <input id="in_%s"><inputExpression id="ie_%s" typeRef="string"><text>%s</text></inputExpression></input>`+"\n", name, name, name)
	}
	b.WriteString(`      <output id="out_Menu" name="Menu" typeRef="string"/>` + "\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, `      <rule id="r%d">`+
			`<inputEntry id="r%d_1"><text>"%s"</text></inputEntry>`+
			`<inputEntry id="r%d_2"><text>"R%d"</text></inputEntry>`+
			`<outputEntry id="r%d_o"><text>"m%d"</text></outputEntry></rule>`+"\n",
			i, i, seasons[i%len(seasons)], i, i, i, i)
	}
	b.WriteString("    </decisionTable>\n  </decision>\n</definitions>\n")
	return b.String()
}

// numericTable: four number inputs, interval cells on the first, n rules, UNIQUE.
func numericTable(n int) string {
	cols := []string{"A", "B", "C", "D"}
	var b strings.Builder
	b.WriteString(header("NumericTable"))
	for _, name := range cols {
		fmt.Fprintf(&b, `  <inputData id="i_%s" name="%s"><variable name="%s" typeRef="number"/></inputData>`+"\n", name, name, name)
	}
	b.WriteString(`  <decision id="d_Grade" name="Grade">` + "\n")
	b.WriteString(`    <variable name="Grade" typeRef="string"/>` + "\n")
	for _, name := range cols {
		fmt.Fprintf(&b, `    <informationRequirement id="ir_%s"><requiredInput href="#i_%s"/></informationRequirement>`+"\n", name, name)
	}
	b.WriteString(`    <decisionTable id="dt_Grade" hitPolicy="UNIQUE">` + "\n")
	for _, name := range cols {
		fmt.Fprintf(&b, `      <input id="in_%s"><inputExpression id="ie_%s" typeRef="number"><text>%s</text></inputExpression></input>`+"\n", name, name, name)
	}
	b.WriteString(`      <output id="out_Grade" name="Grade" typeRef="string"/>` + "\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, `      <rule id="r%d">`+
			`<inputEntry id="r%d_1"><text>[%d..%d]</text></inputEntry>`+
			`<inputEntry id="r%d_2"><text>-</text></inputEntry>`+
			`<inputEntry id="r%d_3"><text>-</text></inputEntry>`+
			`<inputEntry id="r%d_4"><text>-</text></inputEntry>`+
			`<outputEntry id="r%d_o"><text>"g%d"</text></outputEntry></rule>`+"\n",
			i, i, i*10, i*10+9, i, i, i, i, i)
	}
	b.WriteString("    </decisionTable>\n  </decision>\n</definitions>\n")
	return b.String()
}

func main() {
	must(os.WriteFile("models/string-table.dmn", []byte(stringTable(10)), 0o644))
	must(os.WriteFile("models/numeric-table.dmn", []byte(numericTable(10)), 0o644))
	fmt.Println("wrote models/string-table.dmn, models/numeric-table.dmn")
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
