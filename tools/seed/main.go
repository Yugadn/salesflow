// Command seed fills a salesflow stack with realistic demo data through the
// public API, signed in as the administrator: regions, products with
// carton/pack conversions and regional prices, customers, salespeople,
// stocked warehouses, orders left in every stage of the lifecycle, and a
// login for every salesperson plus one for the warehouse.
//
//	docker compose -f deploy/docker-compose.yml up -d --build --wait
//	go run ./tools/seed
//
// Master data is only created once; demo logins are added if missing, so it
// is safe to run again. All names are fictional.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

var (
	api   string
	token string
)

// Demo logins; the admin account comes from the auth service's configuration.
const (
	salesPassword     = "sales12345"
	warehousePassword = "gudang12345"
)

func main() {
	flag.StringVar(&api, "api", "http://localhost:3000/api", "public API base URL (nginx -> gateway)")
	adminEmail := flag.String("admin-email", "admin@salesflow.local", "administrator email")
	adminPassword := flag.String("admin-password", "admin12345", "administrator password")
	flag.Parse()
	log.SetFlags(0)

	token = login(*adminEmail, *adminPassword)

	var regions []struct{ Code string }
	must(call("GET", "/master/regions", nil, &regions))
	seeded := false
	for _, r := range regions {
		seeded = seeded || r.Code == "JKT"
	}
	if seeded {
		log.Println("master data already seeded (region JKT exists)")
	} else {
		seedMasterAndOrders()
	}
	seedUsers()

	log.Println()
	log.Println("Demo logins (http://localhost:3000):")
	log.Printf("  %-28s %-12s %s\n", *adminEmail, *adminPassword, "admin")
	for _, u := range demoUsers {
		log.Printf("  %-28s %-12s %s\n", u.email, u.password, u.role)
	}
}

type demoUser struct {
	email, name, role, salespersonCode, password string
}

var demoUsers = []demoUser{
	{"andi@salesflow.local", "Andi Wijaya", "sales", "S-01", salesPassword},
	{"rina@salesflow.local", "Rina Kartika", "sales", "S-02", salesPassword},
	{"dedi@salesflow.local", "Dedi Kurniawan", "sales", "S-03", salesPassword},
	{"siti@salesflow.local", "Siti Rahma", "sales", "S-04", salesPassword},
	{"gudang@salesflow.local", "Petugas Gudang", "warehouse", "", warehousePassword},
}

// seedUsers creates each demo login that does not exist yet.
func seedUsers() {
	var users []struct{ Email string }
	must(call("GET", "/auth/users", nil, &users))
	exists := map[string]bool{}
	for _, u := range users {
		exists[u.Email] = true
	}
	var salespersons []struct {
		ID   int64
		Code string
	}
	must(call("GET", "/master/salespersons", nil, &salespersons))
	spByCode := map[string]int64{}
	for _, s := range salespersons {
		spByCode[s.Code] = s.ID
	}

	for _, u := range demoUsers {
		if exists[u.email] {
			continue
		}
		body := map[string]any{"email": u.email, "name": u.name, "role": u.role, "password": u.password}
		if u.salespersonCode != "" {
			id, ok := spByCode[u.salespersonCode]
			if !ok {
				log.Printf("skipping %s: salesperson %s not found", u.email, u.salespersonCode)
				continue
			}
			body["salesperson_id"] = id
		}
		must(call("POST", "/auth/users", body, nil))
		log.Printf("created login %s (%s)", u.email, u.role)
	}
}

// login signs in, waiting out the sign-in rate limit if it is hit.
func login(email, password string) string {
	deadline := time.Now().Add(2 * time.Minute)
	for {
		var s struct{ Token string }
		err := call("POST", "/auth/login", map[string]string{"email": email, "password": password}, &s)
		if err == nil {
			return s.Token
		}
		if !errors.Is(err, errRateLimited) || time.Now().After(deadline) {
			log.Fatalf("sign in as %s: %v", email, err)
		}
		time.Sleep(5 * time.Second)
	}
}

func seedMasterAndOrders() {

	// ---- master data
	for _, u := range [][2]string{{"PCS", "Pieces"}, {"PAK", "Pack"}, {"DUS", "Carton"}} {
		_ = call("POST", "/master/uoms", map[string]string{"code": u[0], "name": u[1]}, nil) // may already exist
	}

	region := map[string]int64{}
	for _, r := range [][2]string{{"JKT", "Jakarta"}, {"BDG", "Bandung"}, {"SBY", "Surabaya"}} {
		region[r[0]] = create("/master/regions", map[string]any{"code": r[0], "name": r[1]})
	}

	type product struct {
		sku, name string
		units     map[string]int64             // UoM -> pieces per unit
		prices    map[string]map[string]string // region -> UoM -> price
	}
	products := []product{
		{"TEH-350", "Teh Melati 350ml", map[string]int64{"DUS": 24}, map[string]map[string]string{
			"JKT": {"PCS": "4200", "DUS": "96000"}, "BDG": {"PCS": "4300", "DUS": "98000"}, "SBY": {"PCS": "4100", "DUS": "94000"}}},
		{"KOPI-25", "Kopi Susu Sachet 25g", map[string]int64{"PAK": 10, "DUS": 120}, map[string]map[string]string{
			"JKT": {"PCS": "1500", "PAK": "14000", "DUS": "162000"}, "BDG": {"PCS": "1550", "PAK": "14500", "DUS": "168000"}, "SBY": {"PCS": "1450", "PAK": "13500", "DUS": "156000"}}},
		{"MIE-85", "Mie Goreng 85g", map[string]int64{"DUS": 40}, map[string]map[string]string{
			"JKT": {"PCS": "3100", "DUS": "118000"}, "BDG": {"PCS": "3150", "DUS": "120000"}, "SBY": {"PCS": "3050", "DUS": "116000"}}},
		{"AIR-600", "Air Mineral 600ml", map[string]int64{"DUS": 24}, map[string]map[string]string{
			"JKT": {"PCS": "3000", "DUS": "66000"}, "BDG": {"PCS": "3100", "DUS": "68000"}, "SBY": {"PCS": "2900", "DUS": "64000"}}},
	}
	pid := map[string]int64{}
	for _, p := range products {
		id := create("/master/products", map[string]any{"sku": p.sku, "name": p.name, "base_uom": "PCS"})
		pid[p.sku] = id
		for uom, factor := range p.units {
			must(call("PUT", fmt.Sprintf("/master/products/%d/uoms", id), map[string]any{"uom": uom, "factor": factor}, nil))
		}
		for r, byUoM := range p.prices {
			for uom, price := range byUoM {
				must(call("PUT", fmt.Sprintf("/master/products/%d/prices", id), map[string]any{"region_id": region[r], "uom": uom, "price": price}, nil))
			}
		}
	}

	customer := map[string]int64{}
	for _, c := range []struct {
		code, name, region string
		term               int
	}{
		{"C-JKT-01", "Toko Sumber Rejeki", "JKT", 30},
		{"C-JKT-02", "Minimarket Sinar Jaya", "JKT", 14},
		{"C-BDG-01", "UD Makmur Jaya", "BDG", 30},
		{"C-SBY-01", "CV Berkah Abadi", "SBY", 45},
		{"C-SBY-02", "Toko Sederhana", "SBY", 0},
	} {
		customer[c.code] = create("/master/customers", map[string]any{"code": c.code, "name": c.name, "region_id": region[c.region], "payment_term_days": c.term})
	}

	sales := map[string]int64{}
	for _, s := range [][3]string{{"S-01", "Andi Wijaya", "JKT"}, {"S-02", "Rina Kartika", "JKT"}, {"S-03", "Dedi Kurniawan", "BDG"}, {"S-04", "Siti Rahma", "SBY"}} {
		sales[s[0]] = create("/master/salespersons", map[string]any{"code": s[0], "name": s[1], "region_id": region[s[2]]})
	}

	// ---- warehouses and opening stock (pieces)
	stock := map[string]map[string]int64{
		"JKT": {"TEH-350": 2400, "KOPI-25": 3600, "MIE-85": 2000, "AIR-600": 1200},
		"BDG": {"TEH-350": 960, "KOPI-25": 1200, "MIE-85": 800, "AIR-600": 480},
		"SBY": {"TEH-350": 720, "KOPI-25": 1200, "MIE-85": 400, "AIR-600": 240}, // little water: one order will be refused
	}
	for _, w := range [][2]string{{"JKT", "Gudang Jakarta"}, {"BDG", "Gudang Bandung"}, {"SBY", "Gudang Surabaya"}} {
		id := create("/warehouse/warehouses", map[string]any{"code": "WH-" + w[0], "name": w[1], "region_id": region[w[0]]})
		for sku, qty := range stock[w[0]] {
			must(call("POST", fmt.Sprintf("/warehouse/warehouses/%d/receipts", id), map[string]any{"product_id": pid[sku], "qty": qty}, nil))
		}
	}

	// ---- orders in every stage
	type line struct {
		sku, uom string
		qty      int64
	}
	place := func(cust, sp string, lines ...line) int64 {
		var ls []map[string]any
		for _, l := range lines {
			ls = append(ls, map[string]any{"product_id": pid[l.sku], "uom": l.uom, "qty": l.qty})
		}
		return create("/order/orders", map[string]any{"customer_id": customer[cust], "salesperson_id": sales[sp], "lines": ls})
	}
	confirm := func(order int64, want string) {
		must(call("POST", fmt.Sprintf("/order/orders/%d/confirm", order), nil, nil))
		waitStatus(order, want)
	}
	deliveryOf := func(order int64) int64 {
		var ds []struct{ ID int64 }
		must(call("GET", fmt.Sprintf("/warehouse/deliveries?order_id=%d", order), nil, &ds))
		if len(ds) == 0 {
			log.Fatalf("no delivery for order %d", order)
		}
		return ds[0].ID
	}
	ship := func(order int64) int64 {
		d := deliveryOf(order)
		must(call("POST", fmt.Sprintf("/warehouse/deliveries/%d/ship", d), nil, nil))
		return d
	}

	o1 := place("C-JKT-01", "S-01", line{"TEH-350", "DUS", 10}, line{"KOPI-25", "DUS", 5}, line{"KOPI-25", "PAK", 4})
	confirm(o1, "FULFILLING")
	d1 := ship(o1)
	must(call("POST", fmt.Sprintf("/warehouse/deliveries/%d/deliver", d1), map[string]any{}, nil))
	waitStatus(o1, "INVOICED")
	log.Println("order 1: delivered in full and invoiced")

	o2 := place("C-JKT-02", "S-02", line{"MIE-85", "DUS", 3}, line{"AIR-600", "PCS", 24}, line{"TEH-350", "PCS", 12})
	confirm(o2, "FULFILLING")
	d2 := ship(o2)
	must(call("POST", fmt.Sprintf("/warehouse/deliveries/%d/deliver", d2), map[string]any{"lines": []map[string]any{{"line_no": 1, "delivered_qty": 2}}}, nil))
	waitStatus(o2, "INVOICED")
	log.Println("order 2: one carton refused; invoiced for what arrived")

	o3 := place("C-JKT-01", "S-01", line{"MIE-85", "DUS", 4}, line{"AIR-600", "DUS", 2})
	confirm(o3, "FULFILLING")
	ship(o3)
	log.Println("order 3: shipped, in transit")

	o4 := place("C-BDG-01", "S-03", line{"TEH-350", "DUS", 20}, line{"KOPI-25", "PAK", 15})
	confirm(o4, "FULFILLING")
	log.Println("order 4: stock reserved, waiting to ship")

	o5 := place("C-SBY-01", "S-04", line{"AIR-600", "DUS", 50})
	confirm(o5, "REJECTED")
	log.Println("order 5: refused, Surabaya is short of water")

	place("C-SBY-02", "S-04", line{"KOPI-25", "DUS", 2}, line{"MIE-85", "PCS", 20})
	log.Println("order 6: draft, not confirmed yet")
}

func create(path string, body any) int64 {
	var out struct{ ID int64 }
	must(call("POST", path, body, &out))
	return out.ID
}

func waitStatus(order int64, want string) {
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		var o struct{ Status string }
		must(call("GET", fmt.Sprintf("/order/orders/%d", order), nil, &o))
		if o.Status == want {
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
	log.Fatalf("order %d did not reach %s in time; are the services and RabbitMQ healthy?", order, want)
}

// call sends a JSON request and decodes {"data": ...} into out.
func call(method, path string, body, out any) error {
	var r io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, api+path, r)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w (is the stack running?)", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return errRateLimited
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: %d %s", method, path, resp.StatusCode, raw)
	}
	if out == nil {
		return nil
	}
	var env struct{ Data json.RawMessage }
	if err := json.Unmarshal(raw, &env); err != nil {
		return err
	}
	return json.Unmarshal(env.Data, out)
}

var errRateLimited = errors.New("rate limited")

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
