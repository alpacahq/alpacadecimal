// Command pgogen generates the default.pgo profile shipped at the repository
// root (run via `make pgo-profile`).
//
// It exercises the library with a workload mix modeled on the package's
// design target (see README): overwhelmingly fixed-path values (prices and
// quantities within ±10M at <=12 fractional digits) flowing through parsing,
// arithmetic, rounding, comparison and serialization, plus a small share of
// fallback values so those paths receive samples too.
package main

import (
	"database/sql/driver"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime/pprof"
	"time"

	"github.com/alpacahq/alpacadecimal"
)

// sinks defeat dead-code elimination
var (
	sinkDec   alpacadecimal.Decimal
	sinkStr   string
	sinkBytes []byte
	sinkBool  bool
	sinkInt   int
	sinkValue driver.Value
)

func main() {
	out := flag.String("o", "default.pgo", "output profile path")
	dur := flag.Duration("d", 15*time.Second, "how long to run the workload")
	flag.Parse()

	f, err := os.Create(*out)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	if err := pprof.StartCPUProfile(f); err != nil {
		log.Fatal(err)
	}
	defer pprof.StopCPUProfile()

	prices := []string{
		"187.23", "0.01", "1.005", "12345.6789", "0.000001",
		"999999.999999", "42", "3.14159", "-55.55", "2500",
		"0.25", "18.9999", "7301.12", "150.00", "0.5",
	}
	quantities := []string{
		"1", "10", "100", "0.5", "2.25", "1000", "33", "0.001", "7", "12.5",
	}
	// ~1% of iterations touch fallback values (out of fixed range / >12 digits)
	fallbacks := []string{
		"123456789.123456789", "1.0000000000001", "98765432109876.5",
	}

	jsonDocs := [][]byte{
		[]byte(`"187.23"`), []byte(`42.5`), []byte(`"0.000001"`), []byte(`1.5e3`),
	}

	deadline := time.Now().Add(*dur)
	var iter int
	for {
		// check the clock rarely so it doesn't pollute the profile
		if iter%1024 == 0 && !time.Now().Before(deadline) {
			break
		}
		iter++

		// parsing: API input and DB scans
		price := alpacadecimal.RequireFromString(prices[iter%len(prices)])
		qty := alpacadecimal.RequireFromString(quantities[iter%len(quantities)])
		var scanned alpacadecimal.Decimal
		if err := scanned.Scan([]byte(prices[(iter+3)%len(prices)])); err != nil {
			log.Fatal(err)
		}
		var fromJSON alpacadecimal.Decimal
		if err := fromJSON.UnmarshalJSON(jsonDocs[iter%len(jsonDocs)]); err != nil {
			log.Fatal(err)
		}

		// money arithmetic
		notional := price.Mul(qty)
		total := notional.Add(scanned).Sub(fromJSON)
		avg := alpacadecimal.Avg(price, qty, scanned, notional)
		ratio := total.Div(qty)
		rem := total.Mod(qty)

		// rounding to cents and friends
		cents := notional.Round(2)
		bank := notional.RoundBank(2)
		trunc := ratio.Truncate(4)
		ceil := avg.Ceil()

		// comparisons and predicates
		sinkBool = price.LessThan(qty) || cents.GreaterThanOrEqual(bank) ||
			trunc.IsZero() || ceil.IsPositive() || total.Equal(rem)
		sinkInt = price.Cmp(qty) + total.Sign()

		// serialization: String/SQL/JSON/text/binary
		sinkStr = total.String()
		v, err := cents.Value()
		if err != nil {
			log.Fatal(err)
		}
		sinkValue = v
		b, err := ratio.MarshalJSON()
		if err != nil {
			log.Fatal(err)
		}
		sinkBytes = b
		t, err := avg.MarshalText()
		if err != nil {
			log.Fatal(err)
		}
		sinkBytes = t
		bin, err := rem.MarshalBinary()
		if err != nil {
			log.Fatal(err)
		}
		var back alpacadecimal.Decimal
		if err := back.UnmarshalBinary(bin); err != nil {
			log.Fatal(err)
		}
		sinkDec = back

		// constructors from native types
		sinkDec = alpacadecimal.NewFromInt(int64(iter % 100000)).
			Add(alpacadecimal.NewFromFloat(float64(iter%1000) / 100))
		sinkDec = alpacadecimal.New(int64(iter%100000), -2)

		// occasional fallback traffic (~1%)
		if iter%100 == 0 {
			fb := alpacadecimal.RequireFromString(fallbacks[iter/100%len(fallbacks)])
			sinkDec = fb.Mul(qty).Round(6)
			sinkStr = fb.String()
			sinkBool = fb.GreaterThan(price)
		}
	}

	fmt.Printf("pgogen: %d iterations, profile written to %s\n", iter, *out)
}
