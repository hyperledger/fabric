package clock

import (
	"fmt"
	"testing"
	"time"
)

func TestFakeClockGoldenPath(t *testing.T) {
	clk := NewFake()
	second := NewFake()
	oldT := clk.Now()

	if !clk.Now().Equal(second.Now()) {
		t.Errorf("clocks must start out at the same time but didn't: %#v vs %#v", clk.Now(), second.Now())
	}
	clk.Add(3 * time.Second)
	if clk.Now().Equal(second.Now()) {
		t.Errorf("clocks different must differ: %#v vs %#v", clk.Now(), second.Now())
	}

	clk.Set(oldT)
	if !clk.Now().Equal(second.Now()) {
		t.Errorf("clk should have been been set backwards: %#v vs %#v", clk.Now(), second.Now())
	}

	clk.Sleep(time.Second)
	if clk.Now().Equal(second.Now()) {
		t.Errorf("clk should have been set forwards (by sleeping): %#v vs %#v", clk.Now(), second.Now())
	}
}

func TestNegativeSleep(t *testing.T) {
	clk := NewFake()
	clk.Add(1 * time.Hour)
	first := clk.Now()
	clk.Sleep(-10 * time.Second)
	if !clk.Now().Equal(first) {
		t.Errorf("clk should not move in time on a negative sleep")
	}

}

func ExampleClock() {
	c := Default()
	now := c.Now()
	fmt.Println(now.UTC().Zone())
	// Output:
	// UTC 0
}

func ExampleFakeClock() {
	c := Default()
	fc := NewFake()
	fc.Add(20 * time.Hour)
	fc.Add(-5 * time.Minute) // negatives work, as well

	if fc.Now().Equal(fc.Now()) {
		fmt.Println("FakeClocks' Times always equal themselves.")
	}
	if !c.Now().Equal(fc.Now()) {
		fmt.Println("Clock and FakeClock can be set to different times.")
	}
	if !fc.Now().Equal(NewFake().Now()) {
		fmt.Println("FakeClocks work independently, too.")
	}
	// Output:
	// FakeClocks' Times always equal themselves.
	// Clock and FakeClock can be set to different times.
	// FakeClocks work independently, too.
}
