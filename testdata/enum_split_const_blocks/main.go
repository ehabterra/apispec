// Fixture: one named string type whose constants are declared across SEVERAL
// `const (...)` blocks, which is how any non-trivial enum is actually written —
// grouped by what the values mean, with a comment over each group.
//
// The blocks are deliberately different sizes, and two of them are tied for
// largest. That is the part that matters: the enum used to be the values of the
// BIGGEST block alone, so the size of one group decided whether the others were
// documented at all. Adding a value to a group could therefore replace the whole
// enum with a different group's values — a silent, confident wrong answer, and
// one that shows up as spec drift with no source change to explain it.
//
// Kind is the control: a type whose constants all sit in one block must keep
// documenting exactly those.
package main

import (
	"encoding/json"
	"net/http"
)

// Reason is which rule fired. Every constant below is explicitly typed, so
// belonging to Reason is a FACT about each one — not an inference from which
// block it happens to sit in.
type Reason string

// Creation triggers — 3 values.
const (
	ReasonLessonCompleted Reason = "lesson_completed"
	ReasonHWCompleted     Reason = "hw_completed"
	ReasonQuizAnnounced   Reason = "quiz_announced"
)

// Placement outcomes — 2 values, the smallest block.
const (
	ReasonWithinTarget Reason = "within_target"
	ReasonOverMaximum  Reason = "over_maximum"
)

// Terminal transitions — 3 values, tied with the first block for largest.
const (
	ReasonDeadlinePassed Reason = "deadline_passed"
	ReasonQuizBoundary   Reason = "quiz_boundary"
	ReasonUnenrolled     Reason = "unenrolled"
)

// Kind's constants all live in one block: the control for the case above.
type Kind string

const (
	KindExercise Kind = "exercise"
	KindQuiz     Kind = "quiz"
)

type Candidate struct {
	Reason Reason `json:"reason"`
	Kind   Kind   `json:"kind"`
}

func getCandidate(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(Candidate{Reason: ReasonQuizAnnounced, Kind: KindQuiz})
}

func main() {
	http.HandleFunc("/candidate", getCandidate)
	_ = http.ListenAndServe(":8080", nil)
}
