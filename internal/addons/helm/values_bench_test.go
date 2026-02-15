package helm

import (
	"fmt"
	"testing"
)

// Package-level vars to prevent compiler optimization of benchmark results.
var (
	benchResultValues Values
	benchResultBytes  []byte
	benchResultMap    map[string]interface{}
)

func BenchmarkDeepMerge_SmallMaps(b *testing.B) {
	base := Values{
		"key1": "value1",
		"key2": 42,
		"nested": Values{
			"a": "alpha",
			"b": "bravo",
		},
	}
	override := Values{
		"key2": 99,
		"key3": "new",
		"nested": Values{
			"b": "BRAVO",
			"c": "charlie",
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		benchResultValues = deepMerge(base, override)
	}
}

func BenchmarkDeepMerge_LargeNestedMaps(b *testing.B) {
	base := buildLargeValues(50, 3)
	override := buildLargeValues(50, 3)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		benchResultValues = deepMerge(base, override)
	}
}

func BenchmarkDeepMerge_DeeplyNested(b *testing.B) {
	base := buildDeeplyNestedValues(10)
	override := buildDeeplyNestedValues(10)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		benchResultValues = deepMerge(base, override)
	}
}

func BenchmarkDeepMerge_ThreeMaps(b *testing.B) {
	m1 := buildLargeValues(20, 2)
	m2 := buildLargeValues(20, 2)
	m3 := buildLargeValues(20, 2)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		benchResultValues = deepMerge(m1, m2, m3)
	}
}

func BenchmarkDeepMerge_Parallel(b *testing.B) {
	base := buildLargeValues(30, 3)
	override := buildLargeValues(30, 3)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = deepMerge(base, override)
		}
	})
}

func BenchmarkMerge_SmallMaps(b *testing.B) {
	base := Values{
		"key1": "value1",
		"key2": 42,
		"key3": "value3",
	}
	override := Values{
		"key2": 99,
		"key4": "new",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		benchResultValues = deepMerge(base, override)
	}
}

func BenchmarkMerge_LargeMaps(b *testing.B) {
	base := buildLargeValues(100, 1)
	override := buildLargeValues(100, 1)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		benchResultValues = deepMerge(base, override)
	}
}

func BenchmarkToYAML_SmallValues(b *testing.B) {
	values := Values{
		"key1": "value1",
		"key2": 42,
		"nested": Values{
			"a": "alpha",
			"b": true,
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	var err error
	for b.Loop() {
		benchResultBytes, err = values.toYAML()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkToYAML_LargeValues(b *testing.B) {
	values := buildLargeValues(50, 3)

	b.ReportAllocs()
	b.ResetTimer()
	var err error
	for b.Loop() {
		benchResultBytes, err = values.toYAML()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFromYAML_SmallInput(b *testing.B) {
	yamlData := []byte(`key1: value1
key2: 42
nested:
  a: alpha
  b: true
`)

	b.ReportAllocs()
	b.ResetTimer()
	var err error
	for b.Loop() {
		benchResultValues, err = fromYAML(yamlData)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFromYAML_LargeInput(b *testing.B) {
	values := buildLargeValues(50, 3)
	yamlData, err := values.toYAML()
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		benchResultValues, err = fromYAML(yamlData)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkToMap_SmallValues(b *testing.B) {
	values := Values{
		"key1": "value1",
		"key2": 42,
		"nested": Values{
			"a":    "alpha",
			"list": []string{"one", "two", "three"},
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		benchResultMap = values.ToMap()
	}
}

func BenchmarkToMap_LargeValues(b *testing.B) {
	values := buildLargeValues(50, 3)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		benchResultMap = values.ToMap()
	}
}

func BenchmarkToMap_WithSlices(b *testing.B) {
	values := Values{
		"strings":    []string{"a", "b", "c", "d", "e"},
		"ints":       []int{1, 2, 3, 4, 5},
		"any":        []any{"x", 1, true},
		"valuesList": []Values{{"k1": "v1"}, {"k2": "v2"}},
		"mapList":    []map[string]any{{"k3": "v3"}, {"k4": "v4"}},
		"nested": Values{
			"deep": Values{
				"strings": []string{"deep1", "deep2"},
			},
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		benchResultMap = values.ToMap()
	}
}

func BenchmarkMergeCustomValues_Empty(b *testing.B) {
	base := buildLargeValues(30, 2)
	custom := map[string]any{}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		benchResultValues = MergeCustomValues(base, custom)
	}
}

func BenchmarkMergeCustomValues_WithOverrides(b *testing.B) {
	base := buildLargeValues(30, 2)
	custom := map[string]any{
		"key_0": "override",
		"key_5": Values{
			"sub_0": "override",
		},
		"newkey": "newvalue",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		benchResultValues = MergeCustomValues(base, custom)
	}
}

// buildLargeValues creates a Values map with the given number of keys and nesting depth.
func buildLargeValues(numKeys, depth int) Values {
	v := make(Values, numKeys)
	for i := 0; i < numKeys; i++ {
		key := fmt.Sprintf("key_%d", i)
		if depth > 1 {
			v[key] = buildLargeValues(numKeys/5+1, depth-1)
		} else {
			v[key] = fmt.Sprintf("value_%d", i)
		}
	}
	return v
}

// buildDeeplyNestedValues creates a deeply nested Values map (single chain).
func buildDeeplyNestedValues(depth int) Values {
	if depth <= 0 {
		return Values{"leaf": "value"}
	}
	return Values{
		"level": buildDeeplyNestedValues(depth - 1),
		"data":  fmt.Sprintf("depth_%d", depth),
	}
}
