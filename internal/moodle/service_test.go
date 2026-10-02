package moodle

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const siteInfo = `{"userid":4711,"username":"10001","fullname":"Erika Muster","sitename":"Lernplattform","release":"5.1.3+"}`

const coursesJSON = `[
 {"id":1,"shortname":"TASI23","fullname":"Tutorium Analysis &amp; Stochastik","startdate":1700000000,"enddate":1720000000,"progress":50,"lastaccess":1719000000,"hidden":false,"isfavourite":false},
 {"id":2,"shortname":"I151_I23a","fullname":"I151 - Softwaretechnik","startdate":1780000000,"enddate":1800000000,"progress":null,"lastaccess":0,"hidden":false,"isfavourite":true},
 {"id":3,"shortname":"Orals2026","fullname":"Oral Exams 2026","startdate":1795000000,"enddate":0,"progress":null,"lastaccess":0,"hidden":false,"isfavourite":false}
]`

const contentsJSON = `[
 {"id":10,"section":0,"name":"Allgemeines","summary":"<p>Willkommen &amp; so</p>","uservisible":true,"modules":[
   {"id":100,"name":"Ankündigungen","modname":"forum","url":"https://m/mod/forum/view.php?id=100","uservisible":true,"dates":[]},
   {"id":101,"name":"Folien Kapitel 1","modname":"resource","url":"https://m/mod/resource/view.php?id=101","uservisible":true,"dates":[],
    "contents":[{"type":"file","filename":"kap1.pdf","filesize":2048,"fileurl":"https://m/webservice/pluginfile.php/1/kap1.pdf?forcedownload=1","timemodified":1776933900,"mimetype":"application/pdf"}]},
   {"id":102,"name":"Versteckt","modname":"resource","uservisible":false,"dates":[]},
   {"id":103,"name":"Label","modname":"label","description":"<div>Bitte <b>lesen</b><img src=\"data:image/png;base64,AAAA\"></div>","uservisible":true,"dates":[]}
 ]},
 {"id":11,"section":1,"name":"Woche 1","summary":"","uservisible":true,"modules":[
   {"id":104,"name":"Quiz 1","modname":"quiz","url":"https://m/mod/quiz/view.php?id=104","uservisible":true,
    "dates":[{"label":"Geöffnet:","timestamp":1776933900,"dataid":"timeopen"},{"label":"Geschlossen:","timestamp":1781690340,"dataid":"timeclose"}]}
 ]}
]`

const assignmentsJSON = `{"courses":[{"id":2,"shortname":"I151_I23a","fullname":"I151","assignments":[
  {"id":50,"cmid":500,"course":2,"name":"Alt","duedate":1700000000,"cutoffdate":0,"allowsubmissionsfromdate":0,"intro":"","introattachments":[]},
  {"id":51,"cmid":501,"course":2,"name":"Später","duedate":1795000000,"cutoffdate":0,"allowsubmissionsfromdate":0,"intro":"","introattachments":[]},
  {"id":52,"cmid":502,"course":2,"name":"Bald","duedate":1791000000,"cutoffdate":1791100000,"allowsubmissionsfromdate":0,
   "intro":"<p>Schreibt einen <b>Essay</b></p>","introattachments":[{"filename":"aufgabe.pdf","filesize":10,"fileurl":"https://m/webservice/pluginfile.php/9/aufgabe.pdf","mimetype":"application/pdf","timemodified":1}]},
  {"id":53,"cmid":503,"course":2,"name":"Ohne Frist","duedate":0,"cutoffdate":0,"allowsubmissionsfromdate":0,"intro":"","introattachments":[]}
]}],"warnings":[]}`

func svc(t *testing.T) (*fakeMoodle, *Service) {
	f := newFake(t).on("core_webservice_get_site_info", siteInfo).on("core_enrol_get_users_courses", coursesJSON)
	return f, f.service()
}

// v navigates a JSON-roundtripped result: v(x, "a", 0, "b").
func v(t *testing.T, x any, path ...any) any {
	t.Helper()
	b, err := json.Marshal(x)
	if err != nil {
		t.Fatal(err)
	}
	var cur any
	json.Unmarshal(b, &cur)
	for _, p := range path {
		switch k := p.(type) {
		case string:
			mm, _ := cur.(map[string]any)
			cur = mm[k]
		case int:
			a, _ := cur.([]any)
			if k >= len(a) {
				return nil
			}
			cur = a[k]
		}
	}
	return cur
}

func eq(t *testing.T, got, want any, what string) {
	t.Helper()
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}

func ids(t *testing.T, xs []*M, key string) string {
	var out []string
	for _, x := range xs {
		out = append(out, fmt.Sprint(v(t, x, key)))
	}
	return strings.Join(out, ",")
}

func TestCourses(t *testing.T) {
	f, s := svc(t)
	cs, err := s.Courses("", "all")
	if err != nil || len(cs) != 3 {
		t.Fatalf("%v %d", err, len(cs))
	}
	eq(t, v(t, cs[0], "name"), "Tutorium Analysis & Stochastik", "entity decoding")
	eq(t, f.last().Get("userid"), "4711", "userid param")
	for cls, want := range map[string]string{"current": "2", "past": "1", "future": "3"} {
		c, _ := s.Courses("", cls)
		eq(t, ids(t, c, "id"), want, cls)
	}
	c, _ := s.Courses("softwaretechnik", "all")
	eq(t, ids(t, c, "id"), "2", "query")
	if _, err := s.Courses("", "bogus"); err == nil {
		t.Error("bogus classification accepted")
	}
}

func TestCourseContents(t *testing.T) {
	f, s := svc(t)
	f.on("core_course_get_contents", contentsJSON)
	secs, err := s.CourseContents(7, nil)
	if err != nil || len(secs) != 2 {
		t.Fatalf("%v %d", err, len(secs))
	}
	eq(t, f.last().Get("courseid"), "7", "courseid")
	eq(t, v(t, secs[0], "summary"), "Willkommen & so", "summary")
	mods := v(t, secs[0], "modules").([]any)
	eq(t, len(mods), 3, "hidden module skipped")
	eq(t, v(t, secs[0], "modules", 1, "files", 0, "name"), "kap1.pdf", "file")
	eq(t, v(t, secs[0], "modules", 2, "text"), "Bitte lesen", "label text")
	eq(t, v(t, secs[1], "modules", 0, "dates", "Geschlossen"), "2026-06-17 11:59", "date in Berlin")
	one := int64(1)
	s1, _ := s.CourseContents(7, &one)
	eq(t, len(s1), 1, "section filter")
}

func TestFind(t *testing.T) {
	f, s := svc(t)
	f.on("core_course_get_contents", contentsJSON)
	hits, err := s.Find("kap1", nil)
	if err != nil || len(hits) != 3 {
		t.Fatalf("%v %d", err, len(hits))
	}
	eq(t, v(t, hits[0], "file"), "kap1.pdf", "file hit")
	q, _ := s.Find("quiz", []int64{2})
	eq(t, len(q), 1, "course filter")
	none, _ := s.Find("gibtsnicht", nil)
	eq(t, len(none), 0, "no hits")
}

func TestUpcoming(t *testing.T) {
	f, s := svc(t)
	f.on("core_calendar_get_action_events_by_timesort", `{"events":[{"id":1,"name":"Abgabe 1 ist fällig","modulename":"assign","activityname":"Abgabe 1","timesort":1790776800,
	  "overdue":false,"url":"https://m/mod/assign/view.php?id=5","course":{"id":2,"shortname":"I151_I23a","fullname":"I151"},
	  "action":{"name":"Abgabe hinzufügen","url":"https://m/x","actionable":true}}]}`)
	evs, err := s.Upcoming(14, nil, 50)
	if err != nil || len(evs) != 1 {
		t.Fatal(err)
	}
	eq(t, f.last().Get("timesortfrom"), fakeNow, "from")
	eq(t, f.last().Get("timesortto"), fakeNow+14*86400, "to")
	eq(t, v(t, evs[0], "due"), "2026-09-30 16:00", "due")
	eq(t, v(t, evs[0], "action"), "Abgabe hinzufügen", "action")
}

func TestAssignments(t *testing.T) {
	f, s := svc(t)
	f.on("mod_assign_get_assignments", assignmentsJSON)
	open, _ := s.Assignments(nil, true)
	eq(t, ids(t, open, "assignid"), "52,51,53", "open sorted")
	two := int64(2)
	all, _ := s.Assignments(&two, false)
	eq(t, len(all), 4, "all")
	eq(t, f.last().Get("courseids[0]"), "2", "courseids")

	f.on("mod_assign_get_submission_status", `{"lastattempt":{"submission":{"status":"submitted","timemodified":1790000000},"gradingstatus":"notgraded","graded":false,"locked":false},
	 "feedback":{"gradefordisplay":"15,00 / 20,00","plugins":[{"type":"comments","editorfields":[{"text":"<p>Gut</p>"}]}]},"warnings":[]}`)
	a, err := s.Assignment(52)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, v(t, a, "intro"), "Schreibt einen Essay", "intro")
	eq(t, v(t, a, "attachments", 0, "name"), "aufgabe.pdf", "attachment")
	eq(t, v(t, a, "submission", "status"), "submitted", "status")
	eq(t, v(t, a, "submission", "grade"), "15,00 / 20,00", "grade")
	eq(t, v(t, a, "submission", "feedback"), "Gut", "feedback")
	if _, err := s.Assignment(999); err == nil {
		t.Error("unknown assignment found")
	}
}

func TestOpenAssignmentsWithoutDueDateOnlyFromCurrentCourses(t *testing.T) {
	f, s := svc(t)
	f.on("mod_assign_get_assignments", `{"courses":[
	  {"id":1,"shortname":"TASI23","assignments":[{"id":60,"cmid":600,"name":"Alt ohne Frist","duedate":0,"cutoffdate":0}]},
	  {"id":2,"shortname":"I151_I23a","assignments":[{"id":61,"cmid":601,"name":"Aktuell ohne Frist","duedate":0,"cutoffdate":0}]}]}`)
	open, _ := s.Assignments(nil, true)
	eq(t, ids(t, open, "assignid"), "61", "only current")
}

func TestGrades(t *testing.T) {
	f, s := svc(t)
	f.on("gradereport_overview_get_course_grades", `{"grades":[{"courseid":1,"grade":"127.35","rawgrade":"127.35278"},{"courseid":2,"grade":"-","rawgrade":null}]}`)
	g, _ := s.Grades(nil)
	eq(t, len(g), 1, "ungraded omitted")
	eq(t, v(t, g[0], "course"), "TASI23", "course name")
	f.on("gradereport_user_get_grade_items", `{"usergrades":[{"courseid":6,"gradeitems":[
	  {"itemname":"2nd Artefact","itemtype":"mod","itemmodule":"assign","gradeformatted":"68,00","rangeformatted":"0&ndash;72","percentageformatted":"94,44 %","feedback":"<p>Top</p>","gradedategraded":1729517064},
	  {"itemname":null,"itemtype":"course","gradeformatted":"-","rangeformatted":"0&ndash;100","percentageformatted":"-","feedback":""}]}],"warnings":[]}`)
	six := int64(6)
	items, _ := s.Grades(&six)
	eq(t, v(t, items[0], "range"), "0–72", "range")
	eq(t, v(t, items[0], "feedback"), "Top", "feedback")
	eq(t, v(t, items[1], "item"), "Kurs gesamt", "course total")
}

func TestForums(t *testing.T) {
	f, s := svc(t)
	f.on("mod_forum_get_forums_by_courses", `[{"id":9,"cmid":100,"course":2,"name":"Ankündigungen","numdiscussions":1}]`)
	f.on("mod_forum_get_forum_discussions", `{"discussions":[{"id":70,"discussion":77,"name":"Klausurtermin","userfullname":"Prof X","created":1790000000,"modified":1790000000,
	 "numreplies":2,"pinned":true,"message":"<p>Die Klausur ist am <b>5.10.</b></p>"}],"warnings":[]}`)
	f.on("mod_forum_get_discussion_posts", `{"posts":[{"id":1,"subject":"Klausurtermin","message":"<p>Am 5.10.</p>","author":{"fullname":"Prof X"},"timecreated":1790000000,
	 "attachments":[{"filename":"plan.pdf","fileurl":"https://m/webservice/pluginfile.php/3/plan.pdf","filesize":5,"mimetype":"application/pdf"}]}]}`)
	two := int64(2)
	d, _ := s.ForumDiscussions(&two, nil, 10)
	eq(t, v(t, d[0], "discussionid"), 77, "discussion")
	eq(t, v(t, d[0], "preview"), "Die Klausur ist am 5.10.", "preview")
	p, _ := s.ForumPosts(77)
	eq(t, v(t, p[0], "message"), "Am 5.10.", "message")
	eq(t, v(t, p[0], "attachments", 0, "name"), "plan.pdf", "attachment")
	if _, err := s.ForumDiscussions(nil, nil, 10); err == nil {
		t.Error("no course/forum accepted")
	}
}

func TestNotificationsAndDashboard(t *testing.T) {
	f, s := svc(t)
	f.on("message_popup_get_popup_notifications", `{"notifications":[
	  {"id":1,"subject":"Neue Bewertung","smallmessage":"Note da","fullmessage":"x","read":false,"timecreated":1790000000,"contexturl":"https://m/a","component":"mod_assign"},
	  {"id":2,"subject":"Alt","smallmessage":"alt","read":true,"timecreated":1780000000,"contexturl":null,"component":"mod_forum"}],"unreadcount":1}`)
	all, _ := s.Notifications(20, false)
	unread, _ := s.Notifications(20, true)
	eq(t, len(all), 2, "all")
	eq(t, len(unread), 1, "unread")
	eq(t, f.last().Get("useridto"), "4711", "useridto")

	f.on("message_popup_get_unread_popup_notification_count", "3").on("core_message_get_unread_conversations_count", "1").
		on("core_calendar_get_action_events_by_timesort", `{"events":[{"name":"X endet","timesort":1790700000,"course":{"id":2,"shortname":"I151"}}]}`)
	d, err := s.Dashboard()
	if err != nil {
		t.Fatal(err)
	}
	eq(t, v(t, d, "unread_notifications"), 3, "notif count")
	eq(t, v(t, d, "user"), "Erika Muster", "user")
	eq(t, len(v(t, d, "next_7_days").([]any)), 1, "deadlines")
}

func TestReadFiles(t *testing.T) {
	f, s := svc(t)
	f.file("/webservice/pluginfile.php/5/mod_page/content/index.html", []byte("<html><body><h1>Titel</h1><p>Absatz &amp; mehr</p><script>x()</script></body></html>"))
	txt, err := s.Read(f.srv.URL+"/webservice/pluginfile.php/5/mod_page/content/index.html", 10000)
	if err != nil || !strings.Contains(txt, "Titel") || !strings.Contains(txt, "Absatz & mehr") || strings.Contains(txt, "x()") {
		t.Errorf("html: %q %v", txt, err)
	}
	f.file("/webservice/pluginfile.php/5/folien.pdf", minimalPDF("Vererbung Folie eins", "Polymorphie Folie zwei"))
	txt, err = s.Read(f.srv.URL+"/webservice/pluginfile.php/5/folien.pdf", 10000)
	if err != nil || !strings.Contains(txt, "--- Seite 1 ---") || !strings.Contains(txt, "Vererbung Folie eins") ||
		strings.Index(txt, "--- Seite 2 ---") > strings.Index(txt, "Polymorphie") {
		t.Errorf("pdf: %q %v", txt, err)
	}
	f.file("/webservice/pluginfile.php/5/a.zip", []byte{1, 2})
	if _, err := s.Read(f.srv.URL+"/webservice/pluginfile.php/5/a.zip", 1000); err == nil || !strings.Contains(err.Error(), "moodle_download") {
		t.Errorf("zip: %v", err)
	}
}

func TestPDFGoFallback(t *testing.T) {
	pages, err := goPages(minimalPDF("Hallo Welt", "Zweite"))
	if err != nil || len(pages) != 2 || !strings.Contains(pages[0], "Hallo") {
		t.Errorf("go extractor: %q %v", pages, err)
	}
}

func TestReadOnlyClassification(t *testing.T) {
	for fn, want := range map[string]bool{
		"core_course_get_contents": true, "core_course_search_courses": true, "gradereport_user_get_grade_items": true,
		"mod_assign_save_submission": false, "core_message_send_instant_messages": false,
		"mod_resource_view_resource": false, "tool_mobile_get_autologin_key": false,
	} {
		if IsReadOnly(fn) != want {
			t.Errorf("IsReadOnly(%s) != %v", fn, want)
		}
	}
}

func TestRawCall(t *testing.T) {
	f, s := svc(t)
	f.on("mod_forum_add_discussion_post", `{"postid":1}`).on("core_course_get_categories", `[{"id":1}]`)
	if _, err := s.RawCall("mod_forum_add_discussion_post", map[string]any{"postid": 1}, false); err == nil || !strings.Contains(err.Error(), "confirm") {
		t.Errorf("write without confirm: %v", err)
	}
	if f.callOf("mod_forum_add_discussion_post") != nil {
		t.Fatal("write reached the server")
	}
	if _, err := s.RawCall("mod_forum_add_discussion_post", map[string]any{"postid": 1}, true); err != nil {
		t.Error(err)
	}
	if _, err := s.RawCall("core_course_get_categories", nil, false); err != nil {
		t.Error(err)
	}
}

func TestCourseInfoParticipantsCompletionWhatsNew(t *testing.T) {
	f, s := svc(t)
	f.on("core_course_get_courses_by_field", `{"courses":[{"id":2,"fullname":"I151 - Softwaretechnik","shortname":"I151_I23a","categoryname":"I23",
	  "summary":"<p>Kursbeschreibung</p>","startdate":1780000000,"enddate":1800000000,"format":"topics","contacts":[{"id":8733,"fullname":"Hans-Werner Sehring"}]}]}`)
	info, _ := s.CourseInfo(2)
	eq(t, v(t, info, "teachers"), []any{"Hans-Werner Sehring"}, "teachers")
	eq(t, v(t, info, "summary"), "Kursbeschreibung", "summary")

	f.on("core_enrol_get_enrolled_users", `[{"id":1,"fullname":"Hans-Werner Sehring","lastcourseaccess":1790000000,"roles":[{"shortname":"editingteacher","name":"Trainer/in"}]},
	 {"id":2,"fullname":"Anna Muster","lastcourseaccess":0,"roles":[{"shortname":"student","name":"Teilnehmer/in"}]},
	 {"id":3,"fullname":"Ben Beispiel","roles":[{"shortname":"student","name":"Teilnehmer/in"}]}]`)
	p, _ := s.Participants(2, "", "")
	te, _ := s.Participants(2, "teacher", "")
	an, _ := s.Participants(2, "", "anna")
	eq(t, len(p), 3, "all participants")
	eq(t, len(te), 1, "teachers")
	eq(t, len(an), 1, "name filter")

	f.on("core_course_get_contents", contentsJSON)
	f.on("core_completion_get_activities_completion_status", `{"statuses":[{"cmid":101,"modname":"resource","state":1,"timecompleted":1790000000,"hascompletion":true},
	  {"cmid":104,"modname":"quiz","state":0,"timecompleted":0,"hascompletion":true},{"cmid":100,"modname":"forum","state":0,"hascompletion":false}]}`)
	c, _ := s.Completion(7)
	eq(t, v(t, c, "completed"), "1/2", "completion")
	eq(t, v(t, c, "open", 0, "name"), "Quiz 1", "open name")

	f.on("core_course_get_updates_since", `{"instances":[{"contextlevel":"module","id":100,"updates":[{"name":"discussions","itemids":[1,2]}]},
	  {"contextlevel":"module","id":101,"updates":[{"name":"contentfiles","itemids":[5]},{"name":"configuration","timeupdated":1790600000}]}],"warnings":[]}`)
	news, _ := s.WhatsNew([]int64{2}, 7)
	eq(t, f.callOf("core_course_get_updates_since").Get("since"), fakeNow-7*86400, "since")
	eq(t, len(news), 2, "news")
	eq(t, v(t, news[0], "activity"), "Ankündigungen", "activity")
	eq(t, v(t, news[0], "changes"), []any{"2 neue Diskussionen/Beiträge"}, "changes")
}

func TestQuizzes(t *testing.T) {
	f, s := svc(t)
	f.on("mod_quiz_get_quizzes_by_courses", `{"quizzes":[{"id":20,"coursemodule":200,"course":2,"name":"Test Foliensatz 9","timeopen":1790000000,"timeclose":1790900000,
	  "timelimit":1200,"attempts":2,"grade":10,"sumgrades":5},{"id":21,"coursemodule":201,"course":2,"name":"Alt","timeopen":1700000000,"timeclose":1700100000,"attempts":0,"grade":10,"sumgrades":5}]}`)
	f.on("mod_quiz_get_user_attempts", `{"attempts":[{"id":900,"attempt":1,"state":"finished","timestart":1790100000,"timefinish":1790100600,"sumgrades":4}]}`)
	f.on("mod_quiz_get_user_best_grade", `{"hasgrade":true,"grade":8}`)
	two := int64(2)
	open, err := s.Quizzes(&two, true)
	if err != nil || len(open) != 1 {
		t.Fatalf("%v %d", err, len(open))
	}
	eq(t, v(t, open[0], "time_limit"), "20 min", "limit")
	eq(t, v(t, open[0], "best_grade"), "8 / 10", "best")
	eq(t, v(t, open[0], "my_attempts", 0, "grade"), "8 / 10", "attempt grade")

	f2, s2 := svc(t)
	f2.on("mod_quiz_get_quizzes_by_courses", `{"quizzes":[{"id":20,"course":2,"name":"Q","timeclose":0}]}`)
	s2.Quizzes(nil, false)
	for _, fn := range f2.called() {
		if fn == "mod_quiz_get_user_attempts" {
			t.Error("attempts fetched without course")
		}
	}

	f.on("mod_quiz_get_attempt_review", `{"grade":"8.00","attempt":{"id":900,"state":"finished","sumgrades":4},
	 "questions":[{"slot":1,"type":"multichoice","status":"Richtig","mark":"1,00","maxmark":1,
	   "html":"<div class='qtext'><p>Was ist 2+2?</p></div><div class='answer'>4</div><div class='rightanswer'>Die richtige Antwort ist: 4</div>"}],
	 "additionaldata":[{"id":"feedback","title":"Feedback","content":"<p>Gut gemacht</p>"}]}`)
	r, _ := s.QuizReview(900)
	eq(t, v(t, r, "questions", 0, "mark"), "1,00 / 1", "mark")
	txt := fmt.Sprint(v(t, r, "questions", 0, "text"))
	if !strings.Contains(txt, "Was ist 2+2?") || !strings.Contains(txt, "Die richtige Antwort ist: 4") {
		t.Errorf("review text = %q", txt)
	}
	eq(t, v(t, r, "feedback"), "Gut gemacht", "feedback")
}

func TestChoices(t *testing.T) {
	f, s := svc(t)
	f.on("mod_choice_get_choices_by_courses", `{"choices":[{"id":30,"coursemodule":300,"course":2,"name":"Gruppenabstimmung","intro":"<p>Wählt eine Gruppe</p>",
	  "timeopen":1790000000,"timeclose":1791000000,"allowupdate":true,"allowmultiple":false}]}`)
	f.on("mod_choice_get_choice_options", `{"options":[{"id":1,"text":"Gruppe 1","maxanswers":4,"countanswers":4,"checked":false,"disabled":true},
	  {"id":2,"text":"Gruppe 2","maxanswers":4,"countanswers":1,"checked":true,"disabled":false}]}`)
	cs, _ := s.Choices(2)
	eq(t, v(t, cs[0], "open"), true, "open")
	eq(t, v(t, cs[0], "options", 0, "taken"), "4/4", "taken")
	eq(t, v(t, cs[0], "my_answer"), []any{"Gruppe 2"}, "mine")
}

func TestWritesPreviewFirst(t *testing.T) {
	f, s := svc(t)
	f.on("mod_forum_get_discussion_post", `{"post":{"id":5,"subject":"Klausurtermin","author":{"fullname":"Prof X"},"discussionid":77}}`)
	f.on("mod_forum_add_discussion_post", `{"postid":6,"warnings":[]}`)
	p, _ := s.ForumReply(5, "", "Danke!", false)
	eq(t, v(t, p, "mode"), "preview", "mode")
	eq(t, v(t, p, "subject"), "Re: Klausurtermin", "subject")
	if f.callOf("mod_forum_add_discussion_post") != nil {
		t.Fatal("reply sent without confirm")
	}
	d, _ := s.ForumReply(5, "", "Danke!", true)
	eq(t, v(t, d, "postid"), 6, "postid")
	eq(t, f.last().Get("message"), "Danke!", "message")

	f.on("mod_forum_get_forums_by_courses", `[{"id":9,"course":2,"name":"Fragen"}]`).on("mod_forum_add_discussion", `{"discussionid":88,"warnings":[]}`)
	pp, _ := s.ForumPost(9, "Frage", "Text", false)
	eq(t, v(t, pp, "mode"), "preview", "post preview")
	if f.callOf("mod_forum_add_discussion") != nil {
		t.Fatal("discussion created without confirm")
	}

	f.on("core_message_send_messages_to_conversation", `[{"id":1,"text":"Hi"}]`).on("core_message_send_instant_messages", `[{"msgid":2}]`)
	conv := int64(32559)
	mp, _ := s.SendMessage(&conv, nil, "Hi", false)
	eq(t, v(t, mp, "mode"), "preview", "message preview")
	for _, fn := range f.called() {
		if strings.HasPrefix(fn, "core_message_send") {
			t.Fatal("message sent without confirm")
		}
	}
	s.SendMessage(&conv, nil, "Hi", true)
	eq(t, f.last().Get("messages[0][text]"), "Hi", "conv text")
	u := int64(8733)
	s.SendMessage(nil, &u, "Hallo", true)
	eq(t, f.last().Get("messages[0][touserid]"), "8733", "touserid")
	if _, err := s.SendMessage(nil, nil, "x", true); err == nil {
		t.Error("no recipient accepted")
	}
}

func TestChoiceSubmit(t *testing.T) {
	f, s := svc(t)
	f.on("mod_choice_get_choices_by_courses", `{"choices":[{"id":30,"course":2,"name":"Wahl","allowmultiple":false}]}`)
	f.on("mod_choice_get_choice_options", `{"options":[{"id":1,"text":"A","disabled":false},{"id":2,"text":"B","disabled":true}]}`)
	f.on("mod_choice_submit_choice_response", `{"answers":[{"id":1}],"warnings":[]}`)
	p, _ := s.ChoiceSubmit(30, []int64{1}, false)
	eq(t, v(t, p, "selection"), []any{"A"}, "selection")
	if f.callOf("mod_choice_submit_choice_response") != nil {
		t.Fatal("vote sent without confirm")
	}
	for _, bad := range [][]int64{{7}, {2}, {1, 2}} {
		if _, err := s.ChoiceSubmit(30, bad, true); err == nil {
			t.Errorf("%v accepted", bad)
		}
	}
	s.ChoiceSubmit(30, []int64{1}, true)
	eq(t, f.last().Get("responses[0]"), "1", "response")
}

func TestAssignmentSubmit(t *testing.T) {
	dir := t.TempDir()
	f, s := svc(t)
	f.on("mod_assign_get_assignments", assignmentsJSON)
	f.on("mod_assign_get_submission_status", `{"lastattempt":{"submission":{"status":"draft","plugins":[{"type":"file","fileareas":[{"files":[{"filename":"alt.pdf"}]}]}]},
	 "gradingstatus":"notgraded","cansubmit":true,"canedit":true}}`)
	ab := filepath.Join(dir, "abgabe.pdf")
	os.WriteFile(ab, []byte("pdf"), 0o644)
	p, err := s.AssignmentSubmit(52, []string{ab}, "", false, false)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, v(t, p, "mode"), "preview", "mode")
	if !strings.Contains(fmt.Sprint(v(t, p, "warning")), "alt.pdf") {
		t.Errorf("warning = %v", v(t, p, "warning"))
	}
	if f.callOf("mod_assign_save_submission") != nil || len(f.uploads) != 0 {
		t.Fatal("submission without confirm")
	}

	f.on("mod_assign_get_submission_status", `{"lastattempt":{"submission":{"status":"new"},"canedit":true}}`)
	f.on("mod_assign_save_submission", "[]").on("mod_assign_submit_for_grading", "[]")
	f.on("core_files_get_unused_draft_itemid", `{"itemid":777,"component":"user","filearea":"draft"}`)
	a, b := filepath.Join(dir, "a.pdf"), filepath.Join(dir, "b.txt")
	os.WriteFile(a, []byte("A"), 0o644)
	os.WriteFile(b, []byte("B"), 0o644)
	r, err := s.AssignmentSubmit(52, []string{a, b}, "Mein Text", true, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.uploads) != 2 || f.uploads[0].name != "a.pdf" || f.uploads[0].itemid != f.uploads[1].itemid {
		t.Fatalf("uploads = %+v", f.uploads)
	}
	save := f.callOf("mod_assign_save_submission")
	eq(t, save.Get("plugindata[files_filemanager]"), f.uploads[0].itemid, "draft item")
	eq(t, save.Get("plugindata[onlinetext_editor][text]"), "Mein Text", "online text")
	eq(t, save.Get("plugindata[onlinetext_editor][itemid]"), "777", "text item")
	calls := f.called()
	eq(t, calls[len(calls)-1], "mod_assign_submit_for_grading", "last call")
	eq(t, v(t, r, "result"), "submitted", "result")

	if _, err := s.AssignmentSubmit(52, []string{"/does/not/exist.pdf"}, "", false, true); err == nil {
		t.Error("missing file accepted")
	}
	if _, err := s.AssignmentSubmit(52, nil, "", false, true); err == nil {
		t.Error("empty submission accepted")
	}
}

func TestMarkNotificationsReadAndReadOnly(t *testing.T) {
	f, s := svc(t)
	f.on("core_message_mark_all_notifications_as_read", "true")
	if _, err := s.MarkNotificationsRead(); err != nil {
		t.Fatal(err)
	}
	eq(t, f.last().Get("useridto"), "4711", "useridto")
	s.C.ReadOnly = true
	if _, err := s.MarkNotificationsRead(); !errors.Is(err, ErrReadOnly) {
		t.Errorf("read-only: %v", err)
	}
}

func TestOrderedOutput(t *testing.T) {
	b, _ := json.Marshal(m("z", 1, "a", "", "b", nil, "c", []string{}, "d", true))
	if string(b) != `{"z":1,"d":true}` {
		t.Errorf("M = %s", b)
	}
	if !reflect.DeepEqual(m("x", ptrTrue(false)).Keys(), []string(nil)) {
		t.Error("false pointer kept")
	}
}

func TestMarkMessagesReadOnlyUnreadConversations(t *testing.T) {
	f, s := svc(t)
	f.on("core_message_get_conversations", `{"conversations":[
		{"id":7,"name":"","membercount":2,"unreadcount":2,"members":[{"id":4711,"fullname":"Ich"},{"id":5,"fullname":"Prof. Muster"}],"messages":[{"text":"Hallo","timecreated":1790000000}]},
		{"id":8,"name":"Gruppe I24a","membercount":30,"unreadcount":0,"members":[],"messages":[{"text":"x","timecreated":1790000000}]}]}`)
	f.on("core_message_mark_all_conversation_messages_as_read", "null")
	prev, err := s.MarkMessagesRead(false)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, v(t, prev, "conversations"), "1", "preview count")
	for _, c := range f.called() {
		if c == "core_message_mark_all_conversation_messages_as_read" {
			t.Fatal("preview must not write")
		}
	}
	if _, err := s.MarkMessagesRead(true); err != nil {
		t.Fatal(err)
	}
	eq(t, f.last().Get("conversationid"), "7", "only the unread conversation")
	eq(t, f.last().Get("userid"), "4711", "userid")
	n := 0
	for _, c := range f.called() {
		if c == "core_message_mark_all_conversation_messages_as_read" {
			n++
		}
	}
	eq(t, fmt.Sprint(n), "1", "mark calls")
	s.C.ReadOnly = true
	if _, err := s.MarkMessagesRead(true); !errors.Is(err, ErrReadOnly) {
		t.Errorf("read-only: %v", err)
	}
}
