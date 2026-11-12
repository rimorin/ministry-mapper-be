package handlers

import (
	"fmt"
	"math"
	"slices"
	"testing"
)

// These scenarios are modelled on what the production data looks like rather
// than on minimal cases: territories of ten to twenty maps with neighbours about
// 50m apart, several maps of one block sharing a pin, most links minted by a
// conductor for a group of two to eight within a couple of minutes, a third of
// territories partly finished, and aggregates stored in more than one shape.
// Each one serves a whole group in turn, the way a conductor mints links, and
// states the order the maps are handed out in.

// scenarioMap places a map in metres north and east of the scenario's origin.
type scenarioMap struct {
	id          string
	north, east float64
	completed   int
	total       int
	links       int    // unexpired links already on the map
	aggregates  string // overrides completed/total when the stored shape matters
}

const scenarioLat, scenarioLng = 1.3521, 103.8198

func scenarioPin(north, east float64) (lat, lng float64) {
	const metresPerDegree = 111195.0
	lat = scenarioLat + north/metresPerDegree
	lng = scenarioLng + east/(metresPerDegree*math.Cos(scenarioLat*math.Pi/180))
	return lat, lng
}

func buildScenario(layout []scenarioMap) []MapWithDistance {
	maps := make([]MapWithDistance, len(layout))
	for i, s := range layout {
		lat, lng := scenarioPin(s.north, s.east)
		progress := 0
		if s.total > 0 {
			progress = int(math.Round(float64(s.completed) / float64(s.total) * 100))
		}
		aggregates := s.aggregates
		switch aggregates {
		case "":
			left := s.total - s.completed
			aggregates = fmt.Sprintf(`{"notDone":%d,"notHome":0,"dnc":0,"invalid":0,"done":%d,"completed":%d,"total":%d}`, left, s.completed, s.completed, s.total)
		case "none":
			aggregates = ""
		}
		maps[i] = MapWithDistance{
			ID:          s.id,
			Sequence:    i + 1,
			Progress:    progress,
			AssignCount: s.links,
			Coordinates: fmt.Sprintf(`{"lat":%.7f,"lng":%.7f}`, lat, lng),
			Aggregates:  aggregates,
		}
	}
	return maps
}

// serveGroup hands out one map per publisher in turn from the same spot, each
// new link counting towards its map's workload for the next request, and
// returns the map ids in order together with the furthest anyone was sent.
func serveGroup(t *testing.T, maps []MapWithDistance, north, east float64, threshold, publishers int) ([]string, float64) {
	t.Helper()
	lat, lng := scenarioPin(north, east)
	var order []string
	furthest := 0.0
	for range publishers {
		got := findBestMap(maps, lat, lng, threshold)
		if got == nil {
			t.Fatal("no map selected")
		}
		order = append(order, got.ID)
		furthest = math.Max(furthest, got.Distance)
		got.AssignCount++
	}
	return order, furthest
}

func expectOrder(t *testing.T, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("maps handed out:\n got  %v\n want %v", got, want)
	}
}

// A fresh round in a dense estate. The group fills the maps within 300m of where
// it stands, nearest first, before anyone doubles up; the two maps of block 101
// share a pin and a size and go out in territory order; maps beyond 300m are
// left for a group that starts nearer to them.
func TestScenario_DenseEstateStartOfRound(t *testing.T) {
	maps := buildScenario([]scenarioMap{
		{id: "101A", north: 0, total: 96},
		{id: "101B", north: 0, total: 96},
		{id: "102", north: 40, total: 120, completed: 30},
		{id: "103", east: 60, total: 80},
		{id: "104", north: 150, total: 96, completed: 10},
		{id: "105", north: 280, total: 200},
		{id: "106", north: 350, total: 96},
		{id: "107", north: 600, east: 100, total: 96},
	})
	order, furthest := serveGroup(t, maps, 0, 0, 90, 8)
	expectOrder(t, order, []string{"101A", "101B", "102", "103", "104", "105", "101A", "101B"})
	if furthest > 300 {
		t.Errorf("someone was sent %.0fm, limit is 300m past the nearest map", furthest)
	}
}

// The same estate read back in a different row order must hand out the same
// maps: with shared pins, ties are the norm.
func TestScenario_DenseEstateIsOrderIndependent(t *testing.T) {
	layout := []scenarioMap{
		{id: "101A", north: 0, total: 96},
		{id: "101B", north: 0, total: 96},
		{id: "101C", north: 0, total: 96},
		{id: "102", north: 40, total: 96},
		{id: "103", north: 45, total: 96},
	}
	want, _ := serveGroup(t, buildScenario(layout), 0, 0, 90, 7)
	for start := range layout {
		maps := buildScenario(layout)
		rotated := append(slices.Clone(maps[start:]), maps[:start]...)
		slices.Reverse(rotated)
		got, _ := serveGroup(t, rotated, 0, 0, 90, 7)
		expectOrder(t, got, want)
	}
	expectOrder(t, want, []string{"101A", "101B", "101C", "102", "103", "101A", "101B"})
}

// Late in a round the group meets at a block that is already done. The maps at
// or past the 90% threshold are passed over, leftovers included, and the group
// is spread over the unfinished maps starting from the nearest one.
func TestScenario_EndOfRoundCallerAtFinishedBlock(t *testing.T) {
	maps := buildScenario([]scenarioMap{
		{id: "201", north: 0, total: 100, completed: 100},
		{id: "202", north: 30, total: 100, completed: 97},
		{id: "203", north: 60, total: 100, completed: 92},
		{id: "204", north: 200, total: 100, completed: 60},
		{id: "205", north: 260, east: 50, total: 50, completed: 10},
		{id: "206", north: 450, total: 100},
		{id: "207", north: 470, total: 20, completed: 17},
	})
	order, _ := serveGroup(t, maps, 0, 0, 90, 5)
	expectOrder(t, order, []string{"204", "205", "206", "207", "204"})
}

// One map is left in the territory. The whole group shares it rather than being
// handed finished maps.
func TestScenario_LastUnfinishedMapIsShared(t *testing.T) {
	maps := buildScenario([]scenarioMap{
		{id: "301", north: 0, total: 100, completed: 100},
		{id: "302", north: 50, total: 100, completed: 95},
		{id: "303", north: 120, total: 100, completed: 70},
	})
	order, _ := serveGroup(t, maps, 0, 0, 90, 8)
	expectOrder(t, order, slices.Repeat([]string{"303"}, 8))
}

// Every map is past the threshold. The leftovers are offered again, and the map
// with nothing at all left is never handed out while another has addresses.
func TestScenario_WholeTerritoryPastThreshold(t *testing.T) {
	maps := buildScenario([]scenarioMap{
		{id: "401", north: 0, total: 100, completed: 100},
		{id: "402", north: 20, total: 100, completed: 96},
		{id: "403", north: 45, total: 100, completed: 90},
		{id: "404", north: 200, total: 100, completed: 91},
	})
	order, _ := serveGroup(t, maps, 0, 0, 90, 4)
	expectOrder(t, order, []string{"403", "402", "404", "403"})
}

// Nothing is left anywhere. The request still succeeds, with the nearest map.
func TestScenario_NothingLeftAnywhere(t *testing.T) {
	maps := buildScenario([]scenarioMap{
		{id: "451", north: 80, total: 100, completed: 100},
		{id: "452", north: 0, total: 40, completed: 40},
	})
	order, _ := serveGroup(t, maps, 0, 0, 90, 2)
	expectOrder(t, order, []string{"452", "451"})
}

// A landed territory strung along a road for over a kilometre. The group of six
// shares the three maps within 300m instead of being scattered to the far end.
func TestScenario_SpreadOutTerritoryKeepsGroupTogether(t *testing.T) {
	var layout []scenarioMap
	for i := range 10 {
		layout = append(layout, scenarioMap{id: fmt.Sprintf("5%02d", i+1), north: float64(i * 140), total: 25})
	}
	order, furthest := serveGroup(t, buildScenario(layout), 0, 0, 90, 6)
	expectOrder(t, order, []string{"501", "502", "503", "501", "502", "503"})
	if furthest > 300 {
		t.Errorf("someone was sent %.0fm", furthest)
	}
}

// The same road, with the group starting from the middle: it works outwards in
// both directions.
func TestScenario_SpreadOutTerritoryFromTheMiddle(t *testing.T) {
	var layout []scenarioMap
	for i := range 10 {
		layout = append(layout, scenarioMap{id: fmt.Sprintf("5%02d", i+1), north: float64(i * 140), total: 25})
	}
	order, _ := serveGroup(t, buildScenario(layout), 560, 0, 90, 5)
	expectOrder(t, order, []string{"505", "504", "506", "503", "507"})
}

// One map's pin was dropped 18km away by mistake, as a few are in production.
// It is not handed to a group standing in the estate while nearby maps have
// work, but once it is the only unfinished map the rules do send someone there.
func TestScenario_MisplacedPin(t *testing.T) {
	layout := []scenarioMap{
		{id: "601", north: 0, total: 60},
		{id: "602", north: 40, total: 60},
		{id: "603", north: 18000, total: 60},
	}
	order, _ := serveGroup(t, buildScenario(layout), 0, 0, 90, 4)
	expectOrder(t, order, []string{"601", "602", "601", "602"})

	layout[0].completed, layout[1].completed = 58, 60
	order, furthest := serveGroup(t, buildScenario(layout), 0, 0, 90, 1)
	expectOrder(t, order, []string{"603"})
	if furthest < 17000 {
		t.Errorf("expected the misplaced map at ~18km, got %.0fm", furthest)
	}
}

// A map created today has no aggregates until its first address is worked. It
// is offered like any unfinished map, after the counted maps beside it.
func TestScenario_NewlyCreatedMapWithoutAggregates(t *testing.T) {
	maps := buildScenario([]scenarioMap{
		{id: "701", north: 0, aggregates: "none"},
		{id: "702", north: 0, total: 80},
		{id: "703", north: 30, total: 10, completed: 5},
	})
	order, _ := serveGroup(t, maps, 0, 0, 90, 3)
	expectOrder(t, order, []string{"702", "703", "701"})
}

// Older maps carry per-status counts of zero beside a correct completed/total.
// Such a map has work and must not be mistaken for a finished one; here it is
// the largest in the band and goes first.
func TestScenario_StaleStatusCountsBesideCorrectTotals(t *testing.T) {
	maps := buildScenario([]scenarioMap{
		{id: "751", north: 0, aggregates: `{"notDone":0,"notHome":0,"dnc":0,"invalid":0,"done":0,"completed":0,"total":360}`},
		{id: "752", north: 20, total: 100, completed: 20},
		{id: "753", north: 40, aggregates: `{"completed":12,"total":40}`},
		{id: "754", north: 45, total: 100, completed: 100},
	})
	order, _ := serveGroup(t, maps, 0, 0, 90, 4)
	expectOrder(t, order, []string{"751", "752", "753", "751"})
}

// The conductor mints the links from home, 5km from the territory. The reach is
// still 300m past the nearest map, so the group gets the near edge.
func TestScenario_CallerFarFromTerritory(t *testing.T) {
	maps := buildScenario([]scenarioMap{
		{id: "801", north: 0, total: 60},
		{id: "802", north: 200, total: 60},
		{id: "803", north: 400, total: 60},
		{id: "804", north: 700, total: 60},
	})
	order, _ := serveGroup(t, maps, -5000, 0, 90, 3)
	expectOrder(t, order, []string{"801", "802", "801"})
}

// A second group arrives where another is already working. Idle maps within
// reach go first, then the least busy; the finished map and the one beyond
// 300m are not used.
func TestScenario_SecondGroupJoinsBusyArea(t *testing.T) {
	maps := buildScenario([]scenarioMap{
		{id: "901", north: 0, total: 100, completed: 20, links: 2},
		{id: "902", north: 40, total: 100, completed: 30, links: 1},
		{id: "903", north: 90, total: 100, completed: 97},
		{id: "904", north: 250, total: 100},
		{id: "905", north: 320, total: 100},
	})
	order, _ := serveGroup(t, maps, 0, 0, 90, 3)
	expectOrder(t, order, []string{"904", "902", "904"})
}

// The largest sessions in production are around 35 links. However many are
// minted from one spot, the maps within reach end up within one link of each
// other and nothing else is touched.
func TestScenario_LargeGroupStaysEven(t *testing.T) {
	var layout []scenarioMap
	for i := range 12 {
		layout = append(layout, scenarioMap{id: fmt.Sprintf("B%02d", i+1), north: float64(i * 45), total: 90 + i})
	}
	maps := buildScenario(layout)
	order, _ := serveGroup(t, maps, 0, 0, 90, 35)

	counts := map[string]int{}
	for _, id := range order {
		counts[id]++
	}
	lowest, highest := math.MaxInt, 0
	for i, m := range maps {
		reachable := float64(i*45) <= 300
		if !reachable {
			if counts[m.ID] != 0 {
				t.Errorf("map %s is beyond 300m and got %d links", m.ID, counts[m.ID])
			}
			continue
		}
		lowest, highest = min(lowest, counts[m.ID]), max(highest, counts[m.ID])
	}
	if highest-lowest > 1 {
		t.Errorf("links are uneven across the reachable maps: %v", counts)
	}
}

// Thresholds are per congregation. At 100 the nearly done map is still offered,
// and its 3 leftover addresses rank it behind the fuller map beside it.
func TestScenario_ThresholdOfOneHundred(t *testing.T) {
	layout := []scenarioMap{
		{id: "A", north: 0, total: 100, completed: 97},
		{id: "B", north: 30, total: 100, completed: 40},
		{id: "C", north: 60, total: 100, completed: 100},
	}
	order, _ := serveGroup(t, buildScenario(layout), 0, 0, 100, 3)
	expectOrder(t, order, []string{"B", "A", "B"})

	order, _ = serveGroup(t, buildScenario(layout), 0, 0, 90, 3)
	expectOrder(t, order, []string{"B", "B", "B"})
}
