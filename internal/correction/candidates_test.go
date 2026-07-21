package correction

import (
	"reflect"
	"testing"
)

func TestGenerateCandidates(t *testing.T) {
	got := GenerateCandidates("Scary Movie Cut")
	want := []string{"Scary Movie"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GenerateCandidates = %v, want %v", got, want)
	}
}

func TestGenerateCandidatesNoEditionWord(t *testing.T) {
	got := GenerateCandidates("Some Long Wrong Title")
	want := []string{"Some Long Wrong", "Some Long"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GenerateCandidates = %v, want %v", got, want)
	}
}

func TestGenerateCandidatesSingleTokenInputYieldsNone(t *testing.T) {
	if got := GenerateCandidates("Avatar"); len(got) != 0 {
		t.Errorf("GenerateCandidates = %v, want empty", got)
	}
}