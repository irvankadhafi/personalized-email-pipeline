package main

import (
	"errors"
	"net/http"
	"runtime"
	"strconv"

	"github.com/irvankadhafi/personalized-email-pipeline/internal/campaign"
)

const (
	defaultWebCount = 100_000
	defaultWebSeed  = 7
	maxWebCount     = 1_000_000
)

type evaluationInput struct {
	Count   int64
	Seed    uint64
	Workers int
	Format  campaign.Format
}

type evaluationForm struct {
	Count     string
	Seed      string
	Workers   string
	Format    string
	Errors    map[string]string
	OutOfBand bool
}

func defaultEvaluationForm() evaluationForm {
	return evaluationForm{
		Count: strconv.FormatInt(defaultWebCount, 10), Seed: strconv.FormatUint(defaultWebSeed, 10),
		Workers: strconv.Itoa(runtime.NumCPU()), Format: campaign.TextFormat.String(), Errors: map[string]string{},
	}
}

func parseEvaluationInput(writer http.ResponseWriter, request *http.Request) (evaluationInput, evaluationForm, error) {
	request.Body = http.MaxBytesReader(writer, request.Body, maxWebBody)
	if err := request.ParseForm(); err != nil {
		return evaluationInput{}, evaluationForm{Errors: map[string]string{"form": "invalid form submission"}}, err
	}
	form := evaluationForm{
		Count: request.PostForm.Get("count"), Seed: request.PostForm.Get("seed"), Workers: request.PostForm.Get("workers"),
		Format: request.PostForm.Get("format"), Errors: make(map[string]string),
	}
	for _, field := range []string{"count", "seed", "workers", "format"} {
		if len(request.PostForm[field]) != 1 {
			form.Errors[field] = "enter exactly one value"
		}
	}
	if len(request.Form) != 4 || len(request.PostForm) != 4 {
		form.Errors["form"] = "submit only the four benchmark controls"
	}
	count, countErr := strconv.ParseInt(form.Count, 10, 64)
	if countErr != nil || count < 1 || count > maxWebCount {
		form.Errors["count"] = "enter an integer from 1 through 1000000"
	}
	seed, seedErr := strconv.ParseUint(form.Seed, 10, 64)
	if seedErr != nil {
		form.Errors["seed"] = "enter an unsigned 64-bit integer"
	}
	workers, workersErr := strconv.Atoi(form.Workers)
	if workersErr != nil || workers < 1 || workers > runtime.NumCPU() {
		form.Errors["workers"] = "enter an integer within this machine's logical CPU count"
	}
	format, formatErr := campaign.ParseFormat(form.Format)
	if formatErr != nil || (form.Format != "text" && form.Format != "html") {
		form.Errors["format"] = "select text or html"
	}
	if len(form.Errors) > 0 {
		return evaluationInput{}, form, errors.New("invalid evaluation input")
	}
	return evaluationInput{Count: count, Seed: seed, Workers: workers, Format: format}, form, nil
}
