package goconsumer

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pact-foundation/pact-go/dsl"
	ex "github.com/pact-foundation/pact-go/examples/types"
	"github.com/pact-foundation/pact-go/types"
)

// Common test data
var dir, _ = os.Getwd()
var pactDir = fmt.Sprintf("%s/../../pacts", dir)
var logDir = fmt.Sprintf("%s/log", dir)
var pact dsl.Pact
var form url.Values
var rr http.ResponseWriter
var req *http.Request
var name = "Jean-Marie de La Beaujardière😀😍"
var lastname = "foo fighter"
var addressLine1 = "1640 Riverside Drive"
var city = "Hill Valley"
var state = "California"
var postalCode = "95420-4345"
var password = "issilly"
var like = dsl.Like
var eachLike = dsl.EachLike
var term = dsl.Term

type s = dsl.String
type request = dsl.Request

var loginRequest = ex.LoginRequest{
	Username: name,
	Password: password,
}
var commonHeaders = dsl.MapMatcher{
	"Content-Type": term("application/json; charset=utf-8", `application\/json`),
}

// Use this to control the setup and teardown of Pact
func TestMain(m *testing.M) {
	// Setup Pact and related test stuff
	setup()

	// Run all the tests
	code := m.Run()

	// Shutdown the Mock Service and Write pact files to disk
	pact.WritePact()
	pact.Teardown()

	// Enable when running E2E/integration tests before a release
	if os.Getenv("PACT_INTEGRATED_TESTS") != "" {
		brokerHost := os.Getenv("PACT_BROKER_HOST")
		version := "1.0.0"
		if os.Getenv("TRAVIS_BUILD_NUMBER") != "" {
			version = fmt.Sprintf("1.0.%s-%d", os.Getenv("TRAVIS_BUILD_NUMBER"), time.Now().Unix())
		}

		// Publish the Pacts...
		p := dsl.Publisher{}

		err := p.Publish(types.PublishRequest{
			PactURLs:        []string{filepath.FromSlash(fmt.Sprintf("%s/billy-bobby.json", pactDir))},
			PactBroker:      brokerHost,
			ConsumerVersion: version,
			Tags:            []string{"latest", "sit4"},
			BrokerUsername:  os.Getenv("PACT_BROKER_USERNAME"),
			BrokerPassword:  os.Getenv("PACT_BROKER_PASSWORD"),
		})

		if err != nil {
			log.Println("ERROR: ", err)
		}
	} else {
		log.Println("Skipping publishing")
	}

	os.Exit(code)
}

// Setup common test data
func setup() {
	pact = createPact()

	// Login form values
	form = url.Values{}
	form.Add("username", name)
	form.Add("password", password)

	// Create a request to pass to our handler.
	req, _ = http.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.PostForm = form

	// Record response (satisfies http.ResponseWriter)
	rr = httptest.NewRecorder()
}

// Create Pact connecting to local Daemon
func createPact() dsl.Pact {
	return dsl.Pact{
		Consumer:                 "billy",
		Provider:                 "bobby",
		LogDir:                   logDir,
		PactDir:                  pactDir,
		LogLevel:                 "DEBUG",
		DisableToolValidityCheck: true,
	}
}

func TestPactConsumerLoginHandler_UserExists(t *testing.T) {
	var testBillyExists = func() error {
		client := Client{
			Host: fmt.Sprintf("http://localhost:%d", pact.Server.Port),
		}
		client.loginHandler(rr, req)

		// Expect User to be set on the Client
		if client.user == nil {
			return errors.New("Expected user not to be nil")
		}

		return nil
	}

	body :=
		like(dsl.Matcher{
			"user": dsl.Matcher{
				"name": name,
				"type": term("admin", "admin|user|guest"),
			},
		})

	// Pull from pact broker, used in e2e/integrated tests for pact-go release
	// Setup interactions on the Mock Service. Note that you can have multiple
	// interactions
	pact.
		AddInteraction().
		Given("User billy exists").
		UponReceiving("A request to login with user 'billy'").
		WithRequest(request{
			Method: "POST",
			Path:   term("/users/login/1", "/users/login/[0-9]+"),
			Query: dsl.MapMatcher{
				"foo": term("bar", "[a-zA-Z]+"),
			},
			Body:    dsl.Match(loginRequest),
			Headers: commonHeaders,
		}).
		WillRespondWith(dsl.Response{
			Status: 200,
			Body:   body,
			Headers: dsl.MapMatcher{
				"X-Api-Correlation-Id": dsl.Like("100"),
				"Content-Type":         term("application/json; charset=utf-8", `application\/json`),
			},
		})

	err := pact.Verify(testBillyExists)
	if err != nil {
		t.Fatalf("Error on Verify: %v", err)
	}
}

func TestPactConsumerLoginHandler_UserDoesNotExist(t *testing.T) {
	var testBillyDoesNotExists = func() error {
		client := Client{
			Host: fmt.Sprintf("http://localhost:%d", pact.Server.Port),
		}
		client.loginHandler(rr, req)

		if client.user != nil {
			return fmt.Errorf("Expected user to be nil but in stead got: %v", client.user)
		}

		return nil
	}

	pact.
		AddInteraction().
		Given("User billy does not exist").
		UponReceiving("A request to login with user 'billy'").
		WithRequest(request{
			Method:  "POST",
			Path:    s("/users/login/10"),
			Body:    loginRequest,
			Headers: commonHeaders,
			Query: dsl.MapMatcher{
				"foo": s("anything"),
			},
		}).
		WillRespondWith(dsl.Response{
			Status:  404,
			Headers: commonHeaders,
		})

	err := pact.Verify(testBillyDoesNotExists)
	if err != nil {
		t.Fatalf("Error on Verify: %v", err)
	}
}

func TestPactConsumerLoginHandler_UserUnauthorised(t *testing.T) {
	var testBillyUnauthorized = func() error {
		client := Client{
			Host: fmt.Sprintf("http://localhost:%d", pact.Server.Port),
		}
		client.loginHandler(rr, req)

		if client.user != nil {
			return fmt.Errorf("Expected user to be nil but got: %v", client.user)
		}

		return nil
	}

	pact.
		AddInteraction().
		Given("User billy is unauthorized").
		UponReceiving("A request to login with user 'billy'").
		WithRequest(request{
			Method:  "POST",
			Path:    s("/users/login/10"),
			Body:    loginRequest,
			Headers: commonHeaders,
		}).
		WillRespondWith(dsl.Response{
			Status:  401,
			Headers: commonHeaders,
		})

	err := pact.Verify(testBillyUnauthorized)
	if err != nil {
		t.Fatalf("Error on Verify: %v", err)
	}
}

func TestPactConsumerLoginHandler_UserWithIgnoredFieldsMatchUsingDsl(t *testing.T) {
	var testUserMatchesUsingDsl = func() error {
		client := Client{
			Host: fmt.Sprintf("http://localhost:%d", pact.Server.Port),
		}
		client.loginHandler(rr, req)

		// Expect User to be set on the Client
		if client.user == nil {
			return errors.New("Expected user not to be nil")
		}

		return nil
	}

	body :=
		like(dsl.Matcher{
			"user": dsl.Matcher{
				"name":     like(name),
				"lastname": like(lastname),
				"address": dsl.Matcher{
					"line1":      like(addressLine1),
					"city":       like(city),
					"state":      like(state),
					"postalcode": like(postalCode),
				},
			},
		})

	// Pull from pact broker, used in e2e/integrated tests for pact-go release
	// Setup interactions on the Mock Service. Note that you can have multiple
	// interactions
	pact.
		AddInteraction().
		Given("User billy exists").
		UponReceiving("A request to login with user 'billy' responds with fields matched using DSL").
		WithRequest(request{
			Method: "POST",
			Path:   term("/users/login/1", "/users/login/[0-9]+"),
			Query: dsl.MapMatcher{
				"foo": term("bar", "[a-zA-Z]+"),
			},
			Body:    dsl.Match(loginRequest),
			Headers: commonHeaders,
		}).
		WillRespondWith(dsl.Response{
			Status: 200,
			Body:   body,
			Headers: dsl.MapMatcher{
				"X-Api-Correlation-Id": dsl.Like("100"),
				"Content-Type":         term("application/json; charset=utf-8", `application\/json`),
			},
		})

	err := pact.Verify(testUserMatchesUsingDsl)
	if err != nil {
		t.Fatalf("Error on Verify: %v", err)
	}
}

func TestPactConsumerLoginHandler_UserWithIgnoredFieldsMatchUsingMatcher(t *testing.T) {
	var testUserMatchesUsingDsl = func() error {
		client := Client{
			Host: fmt.Sprintf("http://localhost:%d", pact.Server.Port),
		}
		client.loginHandler(rr, req)

		// Expect User to be set on the Client
		if client.user == nil {
			return errors.New("Expected user not to be nil")
		}

		return nil
	}

	body := dsl.Match(&User{})

	// Pull from pact broker, used in e2e/integrated tests for pact-go release
	// Setup interactions on the Mock Service. Note that you can have multiple
	// interactions
	pact.
		AddInteraction().
		Given("User billy exists").
		UponReceiving("A request to login with user 'billy' responds with fields matched using recursive Match function").
		WithRequest(request{
			Method: "POST",
			Path:   term("/users/login/1", "/users/login/[0-9]+"),
			Query: dsl.MapMatcher{
				"foo": term("bar", "[a-zA-Z]+"),
			},
			Body:    dsl.Match(loginRequest),
			Headers: commonHeaders,
		}).
		WillRespondWith(dsl.Response{
			Status: 200,
			Body:   body,
			Headers: dsl.MapMatcher{
				"X-Api-Correlation-Id": dsl.Like("100"),
				"Content-Type":         term("application/json; charset=utf-8", `application\/json`),
			},
		})

	err := pact.Verify(testUserMatchesUsingDsl)
	if err != nil {
		t.Fatalf("Error on Verify: %v", err)
	}
}
