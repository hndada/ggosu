package osu

import (
	"reflect"
	"strings"
	"testing"
)

func TestNoneSampleSetUsesNormalFallback(t *testing.T) {
	source := "osu file format v14\n[General]\nMode:3\nSampleSet:Normal\n[Difficulty]\nCircleSize:4\n[TimingPoints]\n0,500,4,0,0,100,1,0\n[HitObjects]\n64,192,1000,1,0,0:0:0:0:\n"
	f := Format{}
	want, err := f.Parse([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.Parse([]byte(strings.Replace(source, "SampleSet:Normal", "SampleSet:None", 1)))
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("None changed notes or default hitsounds", err)
	}
}
