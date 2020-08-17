package repo

import (
	"fmt"
	"github.com/badoux/checkmail"
	"github.com/gocql/gocql"
	"github.com/sawickiszymon/gowebapp/models"
	"github.com/sawickiszymon/gowebapp/repo"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"reflect"
)

const (
	INSERT               = `INSERT INTO Email (email, title, content, magic_number) VALUES (?, ?, ?, ?) USING TTL 300`
	SELECT_EMAIL_TO_SEND = `SELECT email, title, content FROM Email WHERE magic_number = ?`
	SELECT_EMAIL         = `SELECT email, title, content, magic_number FROM Email WHERE email = ?`
	SELECT_COUNT         = `SELECT Count(*) FROM Email WHERE email = ?`
	DELETE_MESSAGE       = `DELETE FROM Email WHERE email = ? AND magic_number = ?`
)

var pageState []byte

func NewRepo(s *gocql.Session) repo.PostRepo {
	return &cassandraPostRepo{
		session: s,
	}
}

type cassandraPostRepo struct {
	session *gocql.Session
}

func (s *cassandraPostRepo) Create(e *models.Email) error {

	if isValid := PostRequestValidation(e); !isValid {
		return http.ErrBodyNotAllowed
	}

	err := checkmail.ValidateFormat(e.Email)
	if err != nil {
		log.Fatal(err)
	}

	PostEmail(e, s.session)

	return nil
}

func (s *cassandraPostRepo) SendEmails(magicNumber int) error {
	var emails []models.Email
	e := new(models.Email)

	iter := s.session.Query(SELECT_EMAIL_TO_SEND,
		magicNumber).Iter()

	for iter.Scan(&e.Email, &e.Title, &e.Content) {
		emails = append(emails, *e)
	}
	SendEmails(emails)
	for el := range emails {
		if err := s.session.Query(DELETE_MESSAGE,
			emails[el].Email, magicNumber).Exec(); err != nil {
			log.Fatal(err)
		}
	}
	emails = nil
	return nil
}


func (s *cassandraPostRepo) ViewMessages(pageNumber int, email string) ([]models.Email, error) {

	var emailToDisplay []models.Email
	pageLimit := 4
	e := new(models.Email)

	var numberOfEmails = GetEmailCount(email, s.session)
	var firstRowEmail = (pageNumber * pageLimit) - pageLimit


	if err := s.session.Query(SELECT_EMAIL, email).PageState(pageState).Scan(&e.Email, &e.Title, &e.Content, &e.MagicNumber); err != nil {
		return nil, err
	}

	for i := 0; i < pageNumber; i++ {

		if numberOfEmails <= firstRowEmail {
			fmt.Println(http.ErrBodyNotAllowed)
			return nil, http.ErrBodyNotAllowed
		}

		iter := s.session.Query(SELECT_EMAIL, e.Email).PageState(pageState).PageSize(pageLimit).Iter()

		for iter.Scan(&e.Email, &e.Title, &e.Content, &e.MagicNumber) {
			if pageNumber%2 == 1 && i+1 == pageNumber {
				emailToDisplay = append(emailToDisplay, *e)
			} else if pageNumber%1 == 0 && i+1 == pageNumber {
				emailToDisplay = append(emailToDisplay, *e)
			}
			pageState = iter.PageState()
		}
	}
	pageState = nil

	return emailToDisplay, nil
}

func GetEmailCount(email string, s *gocql.Session) int {
	var count int
	iter := s.Query(SELECT_COUNT, email).Iter()
	for iter.Scan(&count) {
	}
	return count
}

func PostRequestValidation(e *models.Email) bool {
	isValid := true
	v := reflect.Indirect(reflect.ValueOf(e))

	for i := 0; i < v.NumField(); i++ {
		value := v.Field(i)
		if value.IsZero() {
			isValid = false
		}
	}
	return isValid
}

func PostEmail(e *models.Email, session *gocql.Session) {
	if err := session.Query(INSERT, e.Email, e.Title, e.Content, e.MagicNumber).Exec(); err != nil {
		log.Println(err)
	}
}

func SendEmails(e []models.Email) {

	s := NewSmtpConfig()
	addr := s.SmtpAddress + s.SmtpPort
	auth := smtp.PlainAuth(" ", s.SmtpEmail, s.SmtpPass, s.SmtpAddress)

	for _, elem := range e {

		msg := []byte("To:" + elem.Email + "\r\n" +
			"Subject:" + elem.Title + "\r\n" +
			"\r\n" +
			elem.Content + "\r\n")
		to := []string{elem.Email}
		err := smtp.SendMail(addr, auth, s.SmtpEmail, to, msg)
		if err != nil {
			log.Fatal(err)
		}
	}
}

func NewSmtpConfig() *models.SmtpConfig {
	return &models.SmtpConfig{
		SmtpAddress: os.Getenv("SMTP_SERV"),
		SmtpPort:    os.Getenv("SMTP_PORT"),
		SmtpEmail:   os.Getenv("FROM"),
		SmtpPass:    os.Getenv("PASS"),
	}
}
