package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"flag"
	"os"
	"runtime/pprof"

	"github.com/influxdb/influxdb/client/v2"
)

// tag is a struct that contains data
// about a tag for in a series
type AbstractTag struct {
	Key   string `toml:"key"`
	Value string `toml:"value"`
}

type AbstractTags []AbstractTag

func (a AbstractTags) Tag(i int) Tags {
	tags := make(Tags, len(a))

	for j, t := range a {
		tags[j] = Tag{
			Key:   t.Key,
			Value: fmt.Sprintf("%v-%v", t.Value, i),
		}
	}

	return tags
}

func (t AbstractTags) Tagify() string {
	var buf bytes.Buffer
	for i, tag := range t {
		if i == 0 {
			buf.Write([]byte(fmt.Sprintf("%v=%v-%%v,", tag.Key, tag.Value)))
		} else {
			buf.Write([]byte(fmt.Sprintf("%v=%v,", tag.Key, tag.Value)))
		}
	}

	b := buf.Bytes()
	b = b[0 : len(b)-1]

	return string(b)
}

// tag is a struct that contains data
// about a field for in a series
type AbstractField struct {
	Key  string `toml:"key"`
	Type string `toml:"type"`
}

type AbstractFields []AbstractField

func (a AbstractFields) Field() Fields {
	fields := make(Fields, len(a))

	for i, f := range a {
		field := Field{Key: f.Key}
		switch f.Type {
		case "float64":
			field.Value = fmt.Sprintf("%v", rand.Intn(1000))
		case "int":
			field.Value = fmt.Sprintf("%vi", rand.Intn(1000))
		case "bool":
			b := rand.Intn(2) == 1
			field.Value = fmt.Sprintf("%v", b)
		default:
			field.Value = fmt.Sprintf("%v", rand.Intn(1000))
		}

		fields[i] = field
	}

	return fields
}

func (f AbstractFields) Fieldify() (string, []string) {
	var buf bytes.Buffer
	a := make([]string, len(f))
	for i, field := range f {
		buf.Write([]byte(fmt.Sprintf("%v=%%v,", field.Key)))
		a[i] = field.Type
	}

	b := buf.Bytes()
	b = b[0 : len(b)-1]

	return string(b), a
}

///////////////////////////////////////////

// BasicWriter implements the PointGenerator interface
type BasicWriter struct {
	PointCount  int
	Tick        string
	Jitter      bool
	Measurement string
	SeriesCount int
	Tags        AbstractTags
	Fields      AbstractFields
	StartDate   string
	time        time.Time
	mu          sync.Mutex
}

func typeArr(a []string) []interface{} {
	i := make([]interface{}, len(a))
	for j, ty := range a {
		var t string
		switch ty {
		case "float64":
			t = fmt.Sprintf("%v", rand.Intn(1000))
		case "int":
			t = fmt.Sprintf("%vi", rand.Intn(1000))
		case "bool":
			b := rand.Intn(2) == 1
			t = fmt.Sprintf("%t", b)
		default:
			t = fmt.Sprintf("%v", rand.Intn(1000))
		}
		i[j] = t
	}

	return i
}

func (b *BasicWriter) Template() func(i int, t time.Time) *Pnt {
	ts := b.Tags.Tagify()
	fs, fa := b.Fields.Fieldify()
	tmplt := fmt.Sprintf("%v,%v %v %%v", b.Measurement, ts, fs)

	return func(i int, t time.Time) *Pnt {
		p := &Pnt{}
		arr := []interface{}{i}
		arr = append(arr, typeArr(fa)...)
		arr = append(arr, t.UnixNano())

		str := fmt.Sprintf(tmplt, arr...)
		//str := fmt.Sprintf(tmplt, i, rand.Intn(1000), t.UnixNano())
		p.Set([]byte(str))
		return p
	}
}

type Pnt struct {
	line []byte
}

func (p *Pnt) Set(b []byte) {
	p.line = b
}

func (p *Pnt) Next(i int, t time.Time) {
	p.line = []byte(fmt.Sprintf("a,b=c-%v v=%v", i, i))
	//p.line = []byte(fmt.Sprintf("a,b=c-%v v=%v %v", i, rand.Intn(1000), t.UnixNano()))
	//p.line = []byte(fmt.Sprintf("cpu,host=server-%v,location=us-west-%v value=%v %v", i, i, rand.Intn(1000), t.UnixNano()))
}

func (p Pnt) Line() []byte {
	return p.line
}

func (b *BasicWriter) Generate() <-chan Point {
	//c := make(chan Point, 0)
	c := make(chan Point, 15000)
	//tmplt := b.Template()

	go func(c chan Point) {
		defer close(c)

		start, err := time.Parse("2006-Jan-02", b.StartDate)
		if err != nil {
			fmt.Println(err)
		}

		b.mu.Lock()
		b.time = start
		b.mu.Unlock()

		tick, err := time.ParseDuration(b.Tick)
		if err != nil {
			fmt.Println(err)
		}

		for i := 0; i < b.PointCount; i++ {
			b.mu.Lock()
			b.time = b.time.Add(tick)
			b.mu.Unlock()

			for j := 0; j < b.SeriesCount; j++ {
				//p := tmplt(j, b.time)
				p := &Pnt{}
				p.Next(j, b.time)

				c <- *p
			}
		}
	}(c)

	return c
}

// Generate returns a receiving Point
// channel.
func (b *BasicWriter) Generate2() <-chan Point {
	c := make(chan Point, 0)

	go func(c chan Point) {
		defer close(c)

		start, err := time.Parse("2006-Jan-02", b.StartDate)
		if err != nil {
			fmt.Println(err)
		}

		b.mu.Lock()
		b.time = start
		b.mu.Unlock()

		tick, err := time.ParseDuration(b.Tick)
		if err != nil {
			fmt.Println(err)
		}

		for i := 0; i < b.PointCount; i++ {
			b.mu.Lock()
			b.time = b.time.Add(tick)
			b.mu.Unlock()

			for j := 0; j < b.SeriesCount; j++ {
				p := StdPoint{
					Measurement: b.Measurement,
					//Tags:        append(make(Tags, 0), Tag{Key: "host", Value: fmt.Sprintf("server-%v", j)}),
					//Tags:      append(make(Tags, 0), Tag{Key: "host", Value: fmt.Sprintf("server-%v", j)}, Tag{Key: "location", Value: fmt.Sprintf("us-%v", j)}),
					Tags:   b.Tags.Tag(j),    // Bottleneck is here
					Fields: b.Fields.Field(), // Bottleneck is here
					//Fields:    append(make(Fields, 0), Field{Key: "value", Value: fmt.Sprintf("%v", rand.Intn(100))}),
					Timestamp: b.time.UnixNano(),
				}

				c <- p
			}
		}
	}(c)

	return c
}

func (b *BasicWriter) Time() time.Time {
	b.mu.Lock()
	t := b.time
	b.mu.Unlock()
	return t
}

type BasicClient struct {
	Address     string
	Database    string
	Precision   string
	BatchSize   int
	Concurrency int
	SSL         bool
}

// Abstract out more
func (c *BasicClient) Batch(ps <-chan Point, r chan<- response) {
	var buf bytes.Buffer
	var wg sync.WaitGroup

	counter := NewConcurrencyLimiter(c.Concurrency)

	ctr := 0

	for p := range ps {
		b := p.Line()
		ctr++

		buf.Write(b)
		buf.Write([]byte("\n"))

		if ctr%c.BatchSize == 0 && ctr != 0 {
			b := buf.Bytes()

			b = b[0 : len(b)-2]

			wg.Add(1)
			counter.Increment()
			go func(byt []byte) {

				rs := c.send(byt)

				counter.Decrement()
				r <- rs
				wg.Done()
			}(b)

			var temp bytes.Buffer
			buf = temp
		}

	}

	wg.Wait()
}

func post(url string, datatype string, data io.Reader) (*http.Response, error) {

	resp, err := http.Post(url, datatype, data)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(string(body))
	}

	return resp, nil
}

func (c *BasicClient) send(b []byte) response {
	instanceURL := fmt.Sprintf("http://%v/write?db=%v&precision=%v", c.Address, c.Database, c.Precision)
	t := NewTimer()

	t.StartTimer()
	resp, err := post(instanceURL, "application/x-www-form-urlencoded", bytes.NewBuffer(b))
	t.StopTimer()
	if err != nil {
		fmt.Println(err)
	}

	r := response{
		Resp:  resp,
		Time:  time.Now(),
		Timer: t,
	}

	return r
}

func (c *BasicClient) Handle(resp <-chan response, fn func(r response)) {
	for rs := range resp {
		fn(rs)
	}
}

//////////////

type BasicQuery struct {
	Template Query
	time     time.Time
}

func (q *BasicQuery) QueryGenerate() <-chan Query {
	c := make(chan Query, 0)

	go func(chan Query) {
		defer close(c)

		for i := 0; i < 250; i++ {
			time.Sleep(10 * time.Millisecond)
			c <- Query(fmt.Sprintf(string(q.Template), i))
		}

	}(c)

	return c
}

func (q *BasicQuery) SetTime(t time.Time) {
	q.time = t

	return
}

type BasicQueryClient struct {
	Address  string
	Database string
	client   client.Client
}

func (b *BasicQueryClient) Init() {
	u, _ := url.Parse(fmt.Sprintf("http://%v", b.Address))
	cl := client.NewClient(client.Config{
		URL: u,
	})

	b.client = cl
}

func (b *BasicQueryClient) Query(cmd Query, ts time.Time) response {
	q := client.Query{
		Command:  string(cmd),
		Database: b.Database,
	}

	t := NewTimer()

	t.StartTimer()
	_, _ = b.client.Query(q)
	t.StopTimer()

	// Needs actual response type
	r := response{
		Time:  time.Now(),
		Timer: t,
	}

	return r

}

///////////////////

func resetDB(c client.Client, database string) error {
	_, err := c.Query(client.Query{
		Command: fmt.Sprintf("DROP DATABASE %s", database),
	})

	if err != nil && !strings.Contains(err.Error(), "database not found") {
		return err
	}

	_, err = c.Query(client.Query{
		Command: fmt.Sprintf("CREATE DATABASE %s", database),
	})

	return nil
}

type BasicProvisioner struct {
	Address       string
	Database      string
	ResetDatabase bool
}

func (b *BasicProvisioner) Provision() {
	u, _ := url.Parse(fmt.Sprintf("http://%v", b.Address))
	cl := client.NewClient(client.Config{
		URL: u,
	})

	if b.ResetDatabase {
		resetDB(cl, b.Database)
	}
}

func BasicWriteHandler(rs <-chan response, wt *Timer) {
	n := 0
	success := 0
	fail := 0

	s := time.Duration(0)

	for t := range rs {

		n += 1

		if t.Success() {
			success += 1
		} else {
			fail += 1
		}

		s += t.Timer.Elapsed()

	}

	fmt.Printf("Total Requests: %v\n", n)
	fmt.Printf("	Success: %v\n", success)
	fmt.Printf("	Fail: %v\n", fail)
	fmt.Printf("Average Response Time: %v\n", s/time.Duration(n))
	fmt.Printf("Points Per Second: %v\n", float64(n)*float64(10000)/float64(wt.Elapsed().Seconds()))
}

func BasicReadHandler(r <-chan response, rt *Timer) {
	n := 0
	s := time.Duration(0)
	for t := range r {
		n += 1
		s += t.Timer.Elapsed()
	}

	fmt.Printf("Total Queries: %v\n", n)
	fmt.Printf("Average Query Response Time: %v\n", s/time.Duration(n))
}

var (
	cpuprofile = flag.String("cpuprofile", "", "File where cpu profile will be written")
)

func init() {
	flag.Parse()
}

func main() {
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			fmt.Println(err)
			return
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	t := AbstractTags{
		AbstractTag{
			Key:   "host",
			Value: "server",
		},
		//AbstractTag{
		//	Key:   "location",
		//	Value: "us",
		//},
		//AbstractTag{
		//	Key:   "t1",
		//	Value: "t",
		//},
		//AbstractTag{
		//	Key:   "t2",
		//	Value: "t",
		//},
		//AbstractTag{
		//	Key:   "t3",
		//	Value: "t",
		//},
		//AbstractTag{
		//	Key:   "host1",
		//	Value: "server",
		//},
		//AbstractTag{
		//	Key:   "location1",
		//	Value: "us",
		//},
		//AbstractTag{
		//	Key:   "t11",
		//	Value: "t",
		//},
		//AbstractTag{
		//	Key:   "t21",
		//	Value: "t",
		//},
		//AbstractTag{
		//	Key:   "t31",
		//	Value: "t",
		//},
	}

	f := AbstractFields{
		AbstractField{
			Key:  "value",
			Type: "float64",
		},
		//AbstractField{
		//	Key:  "other",
		//	Type: "bool",
		//},
	}

	b := &BasicWriter{
		PointCount:  100,
		SeriesCount: 100000,
		Measurement: "cpu",
		Tags:        t,
		Fields:      f,
		StartDate:   "2006-Jan-02",
		Tick:        "1s",
	}

	//	b2 := &BasicWriter{
	//		PointCount:  100,
	//		SeriesCount: 100000,
	//		Measurement: "mem",
	//		Tags:        t,
	//		Fields:      f,
	//		StartDate:   "2006-Jan-02",
	//		Tick:        "1s",
	//	}

	c := &BasicClient{
		Address: "localhost:1234",
		//Address:     "localhost:8086",
		Database:    "stress",
		Precision:   "n",
		BatchSize:   10000,
		Concurrency: 100,
	}

	w := NewWriter(b, c)
	//	w2 := NewWriter(b2, c)

	qg := &BasicQuery{
		Template: Query("SELECT * FROM cpu WHERE host='server-%v'"),
	}

	qc := &BasicQueryClient{
		Address:  "localhost:8086",
		Database: "stress",
	}

	qc.Init()

	r := NewReader(qg, qc)

	bp := &BasicProvisioner{
		Address:       "localhost:8086",
		Database:      "stress",
		ResetDatabase: true,
	}

	//	bp2 := &BasicProvisioner{
	//		Address:       "localhost:8086",
	//		Database:      "stress",
	//		ResetDatabase: false,
	//	}

	s := NewStressTest(bp, w, r)

	s.Start(BasicWriteHandler, BasicReadHandler)

	//	s2 := NewStressTest(bp2, w2, r)
	//	var wg sync.WaitGroup

	//	wg.Add(1)
	//	go func() {
	//		s.Start(BasicWriteHandler, BasicReadHandler)
	//		wg.Done()
	//	}()
	//
	//	wg.Add(1)
	//	go func() {
	//		s2.Start(BasicWriteHandler, BasicReadHandler)
	//		wg.Done()
	//	}()
	//
	//	wg.Wait()

}
