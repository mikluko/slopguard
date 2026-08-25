package main

import "testing"

// converges is how far two directions fitted from disjoint halves of one
// class's corpus may drift apart before the class is not a class.
//
// The change-event class died at +0.32, having learned the subjects of its
// examples rather than what they had in common, and it read as healthy from
// every other angle until it was measured this way. The floor sits above that
// number and below the classes that survive.
const converges = 0.45

// TestStability fits each class from the even half of its own examples and
// again from the odd half, and compares the two directions.
//
// It measures reproducibility rather than coherence: a class that is two
// coherent things wearing one name reproduces perfectly and passes, because an
// alternating split hands each half the same mixture. Against 200 random
// pseudo-classes carved out of the contract pool this statistic runs to a
// median of +0.06 and a 95th percentile of +0.21, and the classes that survive
// sit at +0.59 and +0.69.
func TestStability(t *testing.T) {
	skipWithoutRuntime(t)
	e, err := model()
	if err != nil {
		t.Fatal(err)
	}
	texts, labels := exemplars()
	vectors, err := e.embed(texts)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range classes {
		var odd, even [][]float32
		var oddLabels, evenLabels []string
		seen := 0
		for i, label := range labels {
			if label == c.name {
				if seen%2 == 0 {
					even, evenLabels = append(even, vectors[i]), append(evenLabels, label)
				} else {
					odd, oddLabels = append(odd, vectors[i]), append(oddLabels, label)
				}
				seen++
				continue
			}
			even, evenLabels = append(even, vectors[i]), append(evenLabels, label)
			odd, oddLabels = append(odd, vectors[i]), append(oddLabels, label)
		}
		first := unit(difference(centroid(even, evenLabels, c.name), centroid(even, evenLabels, "")))
		second := unit(difference(centroid(odd, oddLabels, c.name), centroid(odd, oddLabels, "")))
		agreement := dot(first, second)
		t.Logf("%-10s %3d examples   halves agree at cos %+.3f", c.name, seen, agreement)
		if agreement < converges {
			t.Errorf("%s does not converge: halves agree at %+.3f, below %.2f — it is learning subjects, not a shape",
				c.name, agreement, converges)
		}
	}
}
