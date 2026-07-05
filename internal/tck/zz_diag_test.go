package tck

import (
	"context"
	"testing"
)

func TestDiagC(t *testing.T) {
	base := "/home/user/temis/.tck-corpus/TestCases/compliance-level-3/"
	for _, s := range []string{"0082-feel-coercion/0082-feel-coercion-test-01.xml", "0085-decision-services/0085-decision-services-test-01.xml"} {
		rep, _ := RunFile(context.Background(), nil, base+s)
		t.Logf("=== %s %d/%d", s, rep.Passed(), len(rep.Results))
		for _, c := range rep.Results {
			if c.Pass {
				continue
			}
			if c.Err != nil {
				t.Logf("  FAIL %s ERR %v", c.Case, c.Err)
			} else {
				t.Logf("  FAIL %s got %#v want %#v", c.Case, c.Got, c.Expected)
			}
		}
	}
}
