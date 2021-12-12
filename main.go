package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
)

// --- Creating an array of segments
type Seg struct {
	Seg []Segments `json:"segments"`
}

type Segments struct {
	Duration float64 `json:"duration"`
}

func main() {

	dt := time.Now()
	currentDate := dt.Format("01.02.2006")
	fmt.Printf("Сегодняшняя дата: %s", currentDate)
	apiDate := dt.Format("2006-01-02")

	// --- Creating access to envinroment variables at .env
	e := godotenv.Load()
	if e != nil {
		fmt.Println(e)
	}

	port := os.Getenv("DB_PORT")
	host := os.Getenv("DB_HOST")
	user := os.Getenv("DB_USER")
	dbname := os.Getenv("DB_NAME")
	password := os.Getenv("DB_PASS")
	apiurl := os.Getenv("APIURL")
	imgbbkey := os.Getenv("IMGBB_API_KEY")

	apiurlCurrentDate := string(apiurl + apiDate)

	// --- DB conn & queries
	psqlconn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable", host, port, user, password, dbname)

	db, err := sql.Open("postgres", psqlconn)
	if err != nil {
		panic(err)
	}

	defer db.Close()

	// --- Get API response and write it to the file
	resp, err := http.Get(apiurlCurrentDate)
	check(err)

	defer resp.Body.Close()

	out, err := os.Create("yandex_api.json")
	if err != nil {
		panic(e)
	}

	defer out.Close()
	io.Copy(out, resp.Body)

	// --- JSON Parsing
	jsonFile, err := os.Open("yandex_api.json")
	if err != nil {
		fmt.Println(err)
	}

	defer jsonFile.Close()

	byteValue, _ := ioutil.ReadAll(jsonFile)
	var segms Seg
	json.Unmarshal(byteValue, &segms)

	for i := 0; i < len(segms.Seg); i++ {
		db.Query("insert into apidata (date, duration) values ($1, $2)", currentDate, segms.Seg[i].Duration)
	}

	// --- Working with DB data
	var values plotter.Values
	var duration string
	var durationArr []float64

	rows, err := db.Query("select duration from apidata order by duration DESC")
	if err != nil {
		panic(err)
	}

	for rows.Next() {
		switch durationRows := rows.Scan(&duration); durationRows {
		case sql.ErrNoRows:
			fmt.Println("No rows were returned!")
		case nil:
			parseStringToFloat, err := strconv.ParseFloat(duration, 64)
			if err != nil {
				panic(err)
			}
			convertSecondsToHours := parseStringToFloat / 60 / 60
			durationArr = append(durationArr, convertSecondsToHours)
		default:
			if durationRows != nil {
				panic(durationRows)
			}
		}
	}

	values = durationArr

	countOfRows, err := db.Query("select count(*) from apidata")
	if err != nil {
		panic(err)
	}
	for countOfRows.Next() {
		switch countOfDurationRows := countOfRows.Scan(&duration); countOfDurationRows {
		case sql.ErrNoRows:
			fmt.Println("No rows were returned!")
		case nil:
			s, err := strconv.Atoi(duration)
			if err != nil {
				panic(err)
			}
			fmt.Printf("\nКоличество рейсов сегодня из МОСКВЫ в ПИТЕР: %d", s)
		default:
			if countOfDurationRows != nil {
				panic(countOfDurationRows)
			}
		}
	}

	db.Query("truncate table apidata")

	histPlot(values)

	// --- Creating a POST req to ImgBB via API
	file, err := os.Open("histogram.png")
	if err != nil {
		panic(err)
	}

	defer file.Close()

	r, w := io.Pipe()

	m := multipart.NewWriter(w)

	go func() {
		defer w.Close()
		defer m.Close()

		m.WriteField("key", imgbbkey)
		part, err := m.CreateFormFile("image", "image")
		if err != nil {
			panic(err)
		}

		_, err = io.Copy(part, file)
		if err != nil {
			panic(err)
		}
	}()

	req, err := http.NewRequest(http.MethodPost, "https://api.imgbb.com/1/upload", r)
	if err != nil {
		panic(err)
	}

	req.Header.Add("Content-Type", m.FormDataContentType())

	client := &http.Client{}
	respa, err := client.Do(req)
	if err != nil {
		panic(err)
	}

	ioutil.ReadAll(respa.Body)

}

func check(e error) {
	if e != nil {
		panic(e)
	}
}

// --- Creating a plot of data from DB
func histPlot(values plotter.Values) {
	dt := time.Now()
	currentDate := dt.Format("01.02.2006")
	p := plot.New()

	p.Title.Text = "Время, проведённое в пути с разных станций за " + currentDate + " число"
	p.X.Label.Text = "Количество станций"
	p.Y.Label.Text = "Часы"

	hist, err := plotter.NewBarChart(values, 18)
	if err != nil {
		panic(err)
	}
	p.Add(hist)

	if err := p.Save(20*vg.Centimeter, 20*vg.Centimeter, "histogram.png"); err != nil {
		panic(err)
	}
}
