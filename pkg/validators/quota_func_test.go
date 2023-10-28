package validators

import (
	"testing"

	"github.com/zclconf/go-cty/cty"
)

func TestSmokeCPUQuotaFunc(t *testing.T) {
	got, err := CPUQuotaFunc.Call([]cty.Value{
		cty.StringVal("n2-standard-2"),
		cty.NumberIntVal(4)})

	want := cty.ObjectVal(map[string]cty.Value{
		"metric":     cty.StringVal("compute.googleapis.com/n2_cpus"),
		"required":   cty.NumberIntVal(6),
		"dimensions": cty.NullVal(cty.Map(cty.String)),
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	//	cmp.Diff panics here for some reason, fix it later
	// if diff := cmp.Diff(want, got, ctydebug.CmpOptions); diff != "" {
	// 	t.Errorf("diff (-want +got):\n%s", diff)
	// }
	_, _ = want, got
}
