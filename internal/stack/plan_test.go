package stack

import (
	"slices"
	"strings"
	"testing"
)

func TestPlanStackExamples(t *testing.T) {
	mergedAt := "2026-08-10T00:00:00Z"
	tests := []struct {
		name    string
		remote  []stackPR
		desired []int
		action  stackAction
		delta   []int
		wantErr string
	}{
		{
			name: "merged bottom is ignored",
			remote: []stackPR{
				{Number: 11, MergedAt: &mergedAt},
				{Number: 12},
				{Number: 13},
			},
			desired: []int{12, 13},
			action:  stackNoop,
		},
		{
			name:    "append to active prefix",
			remote:  []stackPR{{Number: 11}, {Number: 12}},
			desired: []int{11, 12, 13},
			action:  stackAppend,
			delta:   []int{13},
		},
		{
			name:    "lower current scope is a no-op",
			remote:  []stackPR{{Number: 11}, {Number: 12}},
			desired: []int{11},
			action:  stackNoop,
		},
		{
			name:    "insert below existing stack",
			remote:  []stackPR{{Number: 11}},
			desired: []int{12, 11},
			action:  stackRestructure,
		},
		{
			name:    "reject dropping open remote PR",
			remote:  []stackPR{{Number: 11}, {Number: 12}},
			desired: []int{13, 11},
			wantErr: "open PRs #12",
		},
		{
			name:    "reject merged PR in desired order",
			remote:  []stackPR{{Number: 11, MergedAt: &mergedAt}, {Number: 12}},
			desired: []int{11, 12},
			wantErr: "merged PR #11",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan, err := planStack(stackResource{Number: 7, PullRequests: test.remote}, test.desired)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("planStack() error = %v, want %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if plan.Action != test.action || !slices.Equal(plan.Delta, test.delta) {
				t.Fatalf("planStack() = %#v, want action %d delta %#v", plan, test.action, test.delta)
			}
		})
	}
}

func FuzzPlanStackProperties(f *testing.F) {
	f.Add([]byte{11, 12, 13}, []byte{12, 13})
	f.Add([]byte{11, 12}, []byte{11, 12, 13})
	f.Add([]byte{11}, []byte{12, 11})
	f.Fuzz(func(t *testing.T, remoteData, desiredData []byte) {
		stack, desired := fuzzStackCase(remoteData, desiredData)
		plan, err := planStack(stack, desired)
		active := stackActiveNumbers(stack)

		if err != nil {
			if isPrefix(desired, active) || isPrefix(active, desired) {
				t.Fatalf("plan rejected a prefix relationship: active=%v desired=%v err=%v", active, desired, err)
			}
			return
		}

		switch plan.Action {
		case stackNoop:
			if !isPrefix(desired, active) {
				t.Fatalf("no-op without desired prefix: active=%v desired=%v", active, desired)
			}
		case stackAppend:
			if !isPrefix(active, desired) || len(plan.Delta) == 0 || !slices.Equal(plan.Delta, desired[len(active):]) {
				t.Fatalf("invalid append plan: active=%v desired=%v delta=%v", active, desired, plan.Delta)
			}
		case stackRestructure:
			if isPrefix(desired, active) || isPrefix(active, desired) {
				t.Fatalf("restructure for a prefix relationship: active=%v desired=%v", active, desired)
			}
		default:
			t.Fatalf("unknown plan action %d", plan.Action)
		}

		updated := applyStackPlan(stack, desired, plan)
		after, err := planStack(updated, desired)
		if err != nil || after.Action != stackNoop {
			t.Fatalf("plan is not idempotent: before=%#v after=%#v err=%v active=%v desired=%v", plan, after, err, stackActiveNumbers(updated), desired)
		}
	})
}

func fuzzStackCase(remoteData, desiredData []byte) (stackResource, []int) {
	var stack stackResource
	seen := make(map[int]bool)
	for _, value := range remoteData {
		number := int(value&0x0f) + 1
		if seen[number] {
			continue
		}
		seen[number] = true
		pr := stackPR{Number: number}
		if value&0x10 != 0 {
			mergedAt := "merged"
			pr.MergedAt = &mergedAt
		}
		stack.PullRequests = append(stack.PullRequests, pr)
		if len(stack.PullRequests) == 8 {
			break
		}
	}

	seen = make(map[int]bool)
	var desired []int
	for _, value := range desiredData {
		number := int(value&0x0f) + 1
		if seen[number] {
			continue
		}
		seen[number] = true
		desired = append(desired, number)
		if len(desired) == 8 {
			break
		}
	}
	return stack, desired
}

func applyStackPlan(stack stackResource, desired []int, plan stackPlan) stackResource {
	switch plan.Action {
	case stackNoop:
		return stack
	case stackAppend:
		for _, number := range plan.Delta {
			stack.PullRequests = append(stack.PullRequests, stackPR{Number: number})
		}
		return stack
	case stackRestructure:
		stack.PullRequests = make([]stackPR, len(desired))
		for i, number := range desired {
			stack.PullRequests[i] = stackPR{Number: number}
		}
		return stack
	default:
		panic("unknown Stack action")
	}
}
