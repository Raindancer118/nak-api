package moodle

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const contentsTTL = 10 * time.Minute

// Service shapes Moodle use cases into compact, ordered JSON objects.
type Service struct {
	C    *Client
	Now  func() time.Time
	Zone *time.Location

	mu       sync.Mutex
	site     *J
	contents map[int64]cached
}

type cached struct {
	at time.Time
	v  J
}

func NewService(c *Client, zone *time.Location) *Service {
	if zone == nil {
		zone = time.UTC
	}
	return &Service{C: c, Now: time.Now, Zone: zone, contents: map[int64]cached{}}
}

// ArgError marks invalid tool arguments.
type ArgError struct{ Msg string }

func (e *ArgError) Error() string { return e.Msg }

func argErr(f string, a ...any) error { return &ArgError{Msg: fmt.Sprintf(f, a...)} }

// ── account ─────────────────────────────────────────────────────────────────

func (s *Service) Whoami() (*M, error) {
	si, err := s.siteInfo()
	if err != nil {
		return nil, err
	}
	return m("user", si.Get("fullname").Str(""), "username", si.Get("username").Str(""), "userid", si.Get("userid").Int(0),
		"site", si.Get("sitename").Str(""), "url", s.C.Base, "moodle_release", si.Get("release").Str(""),
		"available_functions", si.Get("functions").Len()), nil
}

func (s *Service) siteInfo() (J, error) {
	s.mu.Lock()
	if s.site != nil {
		v := *s.site
		s.mu.Unlock()
		return v, nil
	}
	s.mu.Unlock()
	v, err := s.C.Call("core_webservice_get_site_info", nil)
	if err != nil {
		return J{}, err
	}
	s.mu.Lock()
	s.site = &v
	s.mu.Unlock()
	return v, nil
}

func (s *Service) userID() (int64, error) {
	si, err := s.siteInfo()
	if err != nil {
		return 0, err
	}
	return si.Get("userid").Int(0), nil
}

func (s *Service) now() int64 { return s.Now().Unix() }

func (s *Service) time(epoch int64) string {
	if epoch <= 0 {
		return ""
	}
	return time.Unix(epoch, 0).In(s.Zone).Format("2006-01-02 15:04")
}

// ── courses ─────────────────────────────────────────────────────────────────

func (s *Service) enrolled() (J, error) {
	uid, err := s.userID()
	if err != nil {
		return J{}, err
	}
	return s.C.Call("core_enrol_get_users_courses", map[string]any{"userid": uid})
}

func courseState(c J, now int64) string {
	start, end := c.Get("startdate").Int(0), c.Get("enddate").Int(0)
	switch {
	case start > now:
		return "future"
	case end != 0 && end < now:
		return "past"
	}
	return "current"
}

func (s *Service) Courses(query, classification string) ([]*M, error) {
	cls := strings.ToLower(strings.TrimSpace(classification))
	if cls == "" {
		cls = "all"
	}
	switch cls {
	case "all", "current", "past", "future":
	default:
		return nil, argErr("classification must be one of all|current|past|future")
	}
	cs, err := s.enrolled()
	if err != nil {
		return nil, err
	}
	now := s.now()
	out := []*M{}
	for _, c := range cs.Arr() {
		state := courseState(c, now)
		if cls != "all" && cls != state {
			continue
		}
		name, short := Inline(c.Get("fullname").Str("")), Inline(c.Get("shortname").Str(""))
		if !matches(query, name, short) {
			continue
		}
		progress := ""
		if c.Get("progress").IsNumber() {
			progress = fmt.Sprintf("%d%%", c.Get("progress").Int(0))
		}
		id := c.Get("id").Int(0)
		out = append(out, m("id", id, "shortname", short, "name", name, "state", state, "progress", progress,
			"favourite", ptrTrue(c.Get("isfavourite").Bool(false)), "last_access", s.time(c.Get("lastaccess").Int(0)),
			"url", fmt.Sprintf("%s/course/view.php?id=%d", s.C.Base, id)))
	}
	return out, nil
}

func (s *Service) currentCourseIDs() (map[int64]bool, error) {
	cs, err := s.enrolled()
	if err != nil {
		return nil, err
	}
	now := s.now()
	ids := map[int64]bool{}
	for _, c := range cs.Arr() {
		if courseState(c, now) == "current" {
			ids[c.Get("id").Int(0)] = true
		}
	}
	return ids, nil
}

func (s *Service) loadContents(courseID int64, fresh bool) (J, error) {
	s.mu.Lock()
	c, ok := s.contents[courseID]
	s.mu.Unlock()
	if !fresh && ok && s.Now().Before(c.at.Add(contentsTTL)) {
		return c.v, nil
	}
	v, err := s.C.Call("core_course_get_contents", map[string]any{"courseid": courseID})
	if err != nil {
		return J{}, err
	}
	s.mu.Lock()
	s.contents[courseID] = cached{at: s.Now(), v: v}
	s.mu.Unlock()
	return v, nil
}

func (s *Service) CourseContents(courseID int64, section *int64) ([]*M, error) {
	secs, err := s.loadContents(courseID, true)
	if err != nil {
		return nil, err
	}
	out := []*M{}
	for _, sec := range secs.Arr() {
		if section != nil && sec.Get("section").Int(0) != *section {
			continue
		}
		if !sec.Get("uservisible").Bool(true) {
			continue
		}
		mods := []*M{}
		for _, mod := range sec.Get("modules").Arr() {
			if mod.Get("uservisible").Bool(true) {
				mods = append(mods, s.module(mod))
			}
		}
		out = append(out, m("section", sec.Get("section").Int(0), "name", Inline(sec.Get("name").Str("")),
			"summary", Truncate(Blocks(sec.Get("summary").Str("")), 1500), "modules", mods))
	}
	return out, nil
}

var trailingColon = regexp.MustCompile(`:\s*$`)

func (s *Service) module(mod J) *M {
	dates := m()
	for _, d := range mod.Get("dates").Arr() {
		dates.Put(trailingColon.ReplaceAllString(d.Get("label").Str(""), ""), s.time(d.Get("timestamp").Int(0)))
	}
	files := []*M{}
	var links []string
	for _, c := range mod.Get("contents").Arr() {
		switch c.Get("type").Str("") {
		case "file":
			files = append(files, s.file(c))
		case "url":
			links = append(links, c.Get("fileurl").Str(""))
		}
	}
	return m("cmid", mod.Get("id").Int(0), "name", Inline(mod.Get("name").Str("")), "type", mod.Get("modname").Str(""),
		"url", mod.Get("url").Str(""), "dates", dates, "text", Truncate(Blocks(mod.Get("description").Str("")), 1500),
		"files", files, "links", links)
}

func (s *Service) file(f J) *M {
	return m("name", f.Get("filename").Str(""), "size", humanSize(f.Get("filesize").Int(-1)), "mimetype", f.Get("mimetype").Str(""),
		"modified", s.time(f.Get("timemodified").Int(0)), "fileurl", f.Get("fileurl").Str(""))
}

// Find searches module and file names across (selected) enrolled courses.
func (s *Service) Find(query string, courseIDs []int64) ([]*M, error) {
	if strings.TrimSpace(query) == "" {
		return nil, argErr("query is required")
	}
	cs, err := s.enrolled()
	if err != nil {
		return nil, err
	}
	type course struct {
		id    int64
		short string
	}
	var courses []course
	for _, c := range cs.Arr() {
		id := c.Get("id").Int(0)
		if len(courseIDs) == 0 || containsID(courseIDs, id) {
			courses = append(courses, course{id, Inline(c.Get("shortname").Str(""))})
		}
	}
	all := s.loadMany(courseIDsOf(courses, func(c course) int64 { return c.id }))
	hits := []*M{}
	for _, c := range courses {
		secs, ok := all[c.id]
		if !ok {
			continue
		}
		for _, sec := range secs.Arr() {
			section := Inline(sec.Get("name").Str(""))
			for _, mod := range sec.Get("modules").Arr() {
				if !mod.Get("uservisible").Bool(true) {
					continue
				}
				modName := Inline(mod.Get("name").Str(""))
				if matches(query, modName) {
					hit := m("courseid", c.id, "course", c.short, "section", section, "cmid", mod.Get("id").Int(0),
						"module", modName, "type", mod.Get("modname").Str(""), "url", mod.Get("url").Str(""))
					files := []*M{}
					for _, ct := range mod.Get("contents").Arr() {
						if ct.Get("type").Str("") == "file" && len(files) < 5 {
							files = append(files, s.file(ct))
						}
					}
					hit.Put("files", files)
					hits = append(hits, hit)
					continue
				}
				for _, ct := range mod.Get("contents").Arr() {
					if ct.Get("type").Str("") == "file" && matches(query, ct.Get("filename").Str("")) {
						hits = append(hits, m("courseid", c.id, "course", c.short, "section", section, "cmid", mod.Get("id").Int(0),
							"module", modName, "file", ct.Get("filename").Str(""), "size", humanSize(ct.Get("filesize").Int(-1)),
							"fileurl", ct.Get("fileurl").Str("")))
					}
				}
			}
		}
		if len(hits) >= 100 {
			break
		}
	}
	return hits, nil
}

func courseIDsOf[T any](xs []T, f func(T) int64) []int64 {
	out := make([]int64, len(xs))
	for i, x := range xs {
		out[i] = f(x)
	}
	return out
}

// loadMany fetches course contents in parallel (max 6 at once — be polite);
// courses without access are skipped.
func (s *Service) loadMany(ids []int64) map[int64]J {
	out := map[int64]J{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	gate := make(chan struct{}, 6)
	for _, id := range ids {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			gate <- struct{}{}
			defer func() { <-gate }()
			if v, err := s.loadContents(id, false); err == nil {
				mu.Lock()
				out[id] = v
				mu.Unlock()
			}
		}(id)
	}
	wg.Wait()
	return out
}

// ── deadlines & assignments ─────────────────────────────────────────────────

func (s *Service) Upcoming(days int64, courseID *int64, limit int64) ([]*M, error) {
	now := s.now()
	if limit < 1 {
		limit = 1
	}
	if limit > 50 {
		limit = 50
	}
	r, err := s.C.Call("core_calendar_get_action_events_by_timesort", map[string]any{
		"timesortfrom": now, "timesortto": now + days*86400, "limitnum": limit})
	if err != nil {
		return nil, err
	}
	out := []*M{}
	for _, e := range r.Get("events").Arr() {
		if courseID != nil && e.Get("course").Get("id").Int(0) != *courseID {
			continue
		}
		action := e.Get("action")
		act := ""
		if action.Get("actionable").Bool(true) {
			act = action.Get("name").Str("")
		}
		out = append(out, m("event", Inline(e.Get("name").Str("")), "activity", Inline(e.Get("activityname").Str("")),
			"type", e.Get("modulename").Str(""), "course", Inline(e.Get("course").Get("shortname").Str("")),
			"courseid", e.Get("course").Get("id").Int(0), "due", s.time(e.Get("timesort").Int(0)),
			"due_unix", e.Get("timesort").Int(0), "overdue", ptrTrue(e.Get("overdue").Bool(false)), "action", act,
			"url", e.Get("url").Str("")))
	}
	return out, nil
}

func (s *Service) allAssignments(courseID *int64) (J, error) {
	var p map[string]any
	if courseID != nil {
		p = map[string]any{"courseids": []int64{*courseID}}
	}
	return s.C.Call("mod_assign_get_assignments", p)
}

func (s *Service) Assignments(courseID *int64, onlyOpen bool) ([]*M, error) {
	now := s.now()
	current := map[int64]bool{}
	if onlyOpen {
		var err error
		if current, err = s.currentCourseIDs(); err != nil {
			return nil, err
		}
	}
	r, err := s.allAssignments(courseID)
	if err != nil {
		return nil, err
	}
	type row struct {
		key int64
		m   *M
	}
	var rows []row
	for _, c := range r.Get("courses").Arr() {
		course := Inline(c.Get("shortname").Str(""))
		for _, a := range c.Get("assignments").Arr() {
			due, cutoff := a.Get("duedate").Int(0), a.Get("cutoffdate").Int(0)
			open := due >= now || cutoff >= now
			if due == 0 {
				// undated assignments of finished courses are just clutter
				open = current[c.Get("id").Int(0)]
			}
			if onlyOpen && !open {
				continue
			}
			from := a.Get("allowsubmissionsfromdate").Int(0)
			opens := ""
			if from > now {
				opens = s.time(from)
			}
			key := due
			if due == 0 {
				key = 1<<62 - 1
			}
			cmid := a.Get("cmid").Int(0)
			rows = append(rows, row{key, m("assignid", a.Get("id").Int(0), "cmid", cmid, "course", course,
				"courseid", c.Get("id").Int(0), "name", Inline(a.Get("name").Str("")), "due", s.time(due),
				"due_unix", ptrInt(due, due > 0), "cutoff", s.time(cutoff), "opens", opens,
				"url", fmt.Sprintf("%s/mod/assign/view.php?id=%d", s.C.Base, cmid))})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].key < rows[j].key })
	out := make([]*M, len(rows))
	for i, r := range rows {
		out[i] = r.m
	}
	return out, nil
}

func (s *Service) Assignment(assignID int64) (*M, error) {
	r, err := s.allAssignments(nil)
	if err != nil {
		return nil, err
	}
	for _, c := range r.Get("courses").Arr() {
		for _, a := range c.Get("assignments").Arr() {
			if a.Get("id").Int(0) != assignID {
				continue
			}
			atts := []*M{}
			for _, f := range a.Get("introattachments").Arr() {
				atts = append(atts, s.file(f))
			}
			cmid := a.Get("cmid").Int(0)
			grade := a.Get("grade").Int(0)
			out := m("assignid", assignID, "cmid", cmid, "course", Inline(c.Get("shortname").Str("")),
				"name", Inline(a.Get("name").Str("")), "due", s.time(a.Get("duedate").Int(0)),
				"cutoff", s.time(a.Get("cutoffdate").Int(0)), "opens", s.time(a.Get("allowsubmissionsfromdate").Int(0)),
				"max_grade", ptrInt(grade, grade > 0), "team_submission", ptrTrue(a.Get("teamsubmission").Int(0) == 1),
				"intro", Truncate(Blocks(a.Get("intro").Str("")), 6000), "attachments", atts,
				"url", fmt.Sprintf("%s/mod/assign/view.php?id=%d", s.C.Base, cmid))
			if st, err := s.submissionStatus(assignID); err == nil {
				out.Put("submission", st)
			} else {
				out.Put("submission", m("error", err.Error()))
			}
			return out, nil
		}
	}
	return nil, &Error{Code: "notfound", Msg: fmt.Sprintf("Assignment %d not found among your courses", assignID)}
}

func (s *Service) submissionStatus(assignID int64) (*M, error) {
	r, err := s.C.Call("mod_assign_get_submission_status", map[string]any{"assignid": assignID})
	if err != nil {
		return nil, err
	}
	last := r.Get("lastattempt")
	sub := last.Get("submission")
	if !sub.IsObj() {
		sub = last.Get("teamsubmission")
	}
	var files []string
	for _, p := range sub.Get("plugins").Arr() {
		for _, area := range p.Get("fileareas").Arr() {
			for _, f := range area.Get("files").Arr() {
				files = append(files, f.Get("filename").Str(""))
			}
		}
	}
	feedback := ""
	for _, p := range r.Get("feedback").Get("plugins").Arr() {
		if p.Get("type").Str("") == "comments" {
			for _, ef := range p.Get("editorfields").Arr() {
				feedback = Blocks(ef.Get("text").Str(""))
			}
		}
	}
	return m("status", sub.Get("status").Str(""), "last_modified", s.time(sub.Get("timemodified").Int(0)), "files", files,
		"gradingstatus", last.Get("gradingstatus").Str(""), "graded", ptrTrue(last.Get("graded").Bool(false)),
		"locked", ptrTrue(last.Get("locked").Bool(false)), "extension_due", s.time(last.Get("extensionduedate").Int(0)),
		"grade", Inline(r.Get("feedback").Get("gradefordisplay").Str("")), "feedback", feedback), nil
}

// ── grades ──────────────────────────────────────────────────────────────────

func (s *Service) Grades(courseID *int64) ([]*M, error) {
	if courseID == nil {
		cs, err := s.enrolled()
		if err != nil {
			return nil, err
		}
		names := map[int64]string{}
		for _, c := range cs.Arr() {
			names[c.Get("id").Int(0)] = Inline(c.Get("shortname").Str(""))
		}
		r, err := s.C.Call("gradereport_overview_get_course_grades", nil)
		if err != nil {
			return nil, err
		}
		out := []*M{}
		for _, g := range r.Get("grades").Arr() {
			grade := strings.TrimSpace(g.Get("grade").Str("-"))
			if grade == "" || grade == "-" {
				continue
			}
			id := g.Get("courseid").Int(0)
			out = append(out, m("courseid", id, "course", names[id], "grade", grade))
		}
		return out, nil
	}
	uid, err := s.userID()
	if err != nil {
		return nil, err
	}
	r, err := s.C.Call("gradereport_user_get_grade_items", map[string]any{"courseid": *courseID, "userid": uid})
	if err != nil {
		return nil, err
	}
	out := []*M{}
	for _, ug := range r.Get("usergrades").Arr() {
		for _, it := range ug.Get("gradeitems").Arr() {
			typ := it.Get("itemtype").Str("")
			name := strings.TrimSpace(it.Get("itemname").Str(""))
			if name == "" {
				switch typ {
				case "course":
					name = "Kurs gesamt"
				case "category":
					name = "Kategorie gesamt"
				default:
					name = typ
				}
			}
			out = append(out, m("item", Inline(name), "type", it.Get("itemmodule").Str(typ),
				"grade", Inline(it.Get("gradeformatted").Str("")), "range", Inline(it.Get("rangeformatted").Str("")),
				"percent", Inline(it.Get("percentageformatted").Str("")), "graded", s.time(it.Get("gradedategraded").Int(0)),
				"feedback", Blocks(it.Get("feedback").Str(""))))
		}
	}
	return out, nil
}

// ── forums & messages ───────────────────────────────────────────────────────

func (s *Service) ForumDiscussions(courseID, forumID *int64, perForum int64) ([]*M, error) {
	if courseID == nil && forumID == nil {
		return nil, argErr("courseid or forumid is required")
	}
	type forum struct {
		id   int64
		name string
	}
	var forums []forum
	if forumID != nil {
		forums = append(forums, forum{id: *forumID})
	} else {
		r, err := s.C.Call("mod_forum_get_forums_by_courses", map[string]any{"courseids": []int64{*courseID}})
		if err != nil {
			return nil, err
		}
		for _, f := range r.Arr() {
			forums = append(forums, forum{f.Get("id").Int(0), Inline(f.Get("name").Str(""))})
		}
	}
	if perForum < 1 {
		perForum = 1
	}
	out := []*M{}
	for _, f := range forums {
		r, err := s.C.Call("mod_forum_get_forum_discussions", map[string]any{"forumid": f.id, "page": 0, "perpage": perForum})
		if err != nil {
			return nil, err
		}
		for _, d := range r.Get("discussions").Arr() {
			unread := d.Get("numunread").Int(0)
			lastAct := d.Get("timemodified").Int(d.Get("modified").Int(0))
			out = append(out, m("forum", f.name, "forumid", f.id, "discussionid", d.Get("discussion").Int(0),
				"subject", Inline(d.Get("name").Str(d.Get("subject").Str(""))), "author", d.Get("userfullname").Str(""),
				"created", s.time(d.Get("created").Int(0)), "last_activity", s.time(lastAct),
				"replies", d.Get("numreplies").Int(0), "unread", ptrInt(unread, unread > 0), "pinned", ptrTrue(d.Get("pinned").Bool(false)),
				"preview", InlineMax(d.Get("message").Str(""), 300)))
		}
	}
	return out, nil
}

func (s *Service) ForumPosts(discussionID int64) ([]*M, error) {
	r, err := s.C.Call("mod_forum_get_discussion_posts", map[string]any{"discussionid": discussionID, "sortby": "created", "sortdirection": "ASC"})
	if err != nil {
		return nil, err
	}
	out := []*M{}
	for _, p := range r.Get("posts").Arr() {
		att := []*M{}
		for _, f := range p.Get("attachments").Arr() {
			att = append(att, s.file(f))
		}
		out = append(out, m("postid", p.Get("id").Int(0), "reply_to", ptrInt(p.Get("parentid").Int(0), p.Get("parentid").IsNumber()),
			"subject", Inline(p.Get("subject").Str("")), "author", p.Get("author").Get("fullname").Str(""),
			"time", s.time(p.Get("timecreated").Int(0)), "message", Truncate(Blocks(p.Get("message").Str("")), 8000), "attachments", att))
	}
	return out, nil
}

func (s *Service) Notifications(limit int64, unreadOnly bool) ([]*M, error) {
	uid, err := s.userID()
	if err != nil {
		return nil, err
	}
	if limit < 1 {
		limit = 1
	}
	r, err := s.C.Call("message_popup_get_popup_notifications", map[string]any{"useridto": uid, "newestfirst": true, "limit": limit, "offset": 0})
	if err != nil {
		return nil, err
	}
	out := []*M{}
	for _, n := range r.Get("notifications").Arr() {
		read := n.Get("read").Bool(false)
		if unreadOnly && read {
			continue
		}
		text := n.Get("smallmessage").Str("")
		if strings.TrimSpace(text) == "" {
			text = n.Get("fullmessage").Str("")
		}
		out = append(out, m("id", n.Get("id").Int(0), "subject", Inline(n.Get("subject").Str("")),
			"text", Truncate(Blocks(text), 1500), "time", s.time(n.Get("timecreated").Int(0)), "unread", ptrTrue(!read),
			"from", n.Get("component").Str(""), "url", n.Get("contexturl").Str("")))
	}
	return out, nil
}

func (s *Service) Conversations(limit int64) ([]*M, error) {
	me, err := s.userID()
	if err != nil {
		return nil, err
	}
	if limit < 1 {
		limit = 1
	}
	r, err := s.C.Call("core_message_get_conversations", map[string]any{"userid": me, "limitfrom": 0, "limitnum": limit})
	if err != nil {
		return nil, err
	}
	out := []*M{}
	for _, c := range r.Get("conversations").Arr() {
		var members []string
		for _, mem := range c.Get("members").Arr() {
			if mem.Get("id").Int(0) != me {
				members = append(members, mem.Get("fullname").Str(""))
			}
		}
		last := c.Get("messages").Idx(0)
		name := strings.TrimSpace(c.Get("name").Str(""))
		if name == "" {
			name = strings.Join(members, ", ")
		} else {
			name = Inline(name)
		}
		unread := c.Get("unreadcount").Int(0)
		out = append(out, m("conversationid", c.Get("id").Int(0), "name", name, "members", c.Get("membercount").Int(0),
			"unread", ptrInt(unread, unread > 0), "last_message", InlineMax(last.Get("text").Str(""), 300),
			"last_time", s.time(last.Get("timecreated").Int(0))))
	}
	return out, nil
}

func (s *Service) ConversationMessages(convID, limit int64) ([]*M, error) {
	me, err := s.userID()
	if err != nil {
		return nil, err
	}
	if limit < 1 {
		limit = 1
	}
	r, err := s.C.Call("core_message_get_conversation_messages", map[string]any{
		"currentuserid": me, "convid": convID, "limitfrom": 0, "limitnum": limit, "newest": true})
	if err != nil {
		return nil, err
	}
	names := map[int64]string{}
	for _, mem := range r.Get("members").Arr() {
		names[mem.Get("id").Int(0)] = mem.Get("fullname").Str("")
	}
	msgs := r.Get("messages").Arr()
	out := make([]*M, 0, len(msgs))
	for i := len(msgs) - 1; i >= 0; i-- { // API returns newest first
		msg := msgs[i]
		from := names[msg.Get("useridfrom").Int(0)]
		if from == "" {
			from = fmt.Sprint(msg.Get("useridfrom").Int(0))
		}
		out = append(out, m("from", from, "time", s.time(msg.Get("timecreated").Int(0)), "text", Blocks(msg.Get("text").Str(""))))
	}
	return out, nil
}

// ── overview ────────────────────────────────────────────────────────────────

func (s *Service) Dashboard() (*M, error) {
	me, err := s.userID()
	if err != nil {
		return nil, err
	}
	si, _ := s.siteInfo()
	n, err := s.C.Call("message_popup_get_unread_popup_notification_count", map[string]any{"useridto": me})
	if err != nil {
		return nil, err
	}
	cv, err := s.C.Call("core_message_get_unread_conversations_count", map[string]any{"useridto": me})
	if err != nil {
		return nil, err
	}
	up, err := s.Upcoming(7, nil, 20)
	if err != nil {
		return nil, err
	}
	return m("user", si.Get("fullname").Str(""), "unread_notifications", n.Int(0), "unread_conversations", cv.Int(0), "next_7_days", up), nil
}

func (s *Service) CourseInfo(courseID int64) (*M, error) {
	r, err := s.C.Call("core_course_get_courses_by_field", map[string]any{"field": "id", "value": courseID})
	if err != nil {
		return nil, err
	}
	c := r.Get("courses").Idx(0)
	if c.IsNull() {
		return nil, &Error{Code: "notfound", Msg: fmt.Sprintf("Course %d not found", courseID)}
	}
	var teachers []string
	for _, t := range c.Get("contacts").Arr() {
		teachers = append(teachers, t.Get("fullname").Str(""))
	}
	return m("id", courseID, "shortname", Inline(c.Get("shortname").Str("")), "name", Inline(c.Get("fullname").Str("")),
		"category", Inline(c.Get("categoryname").Str("")), "teachers", teachers, "start", s.time(c.Get("startdate").Int(0)),
		"end", s.time(c.Get("enddate").Int(0)), "format", c.Get("format").Str(""),
		"summary", Truncate(Blocks(c.Get("summary").Str("")), 4000), "url", fmt.Sprintf("%s/course/view.php?id=%d", s.C.Base, courseID)), nil
}

func (s *Service) Participants(courseID int64, role, query string) ([]*M, error) {
	r, err := s.C.Call("core_enrol_get_enrolled_users", map[string]any{"courseid": courseID})
	if err != nil {
		return nil, err
	}
	out := []*M{}
	for _, u := range r.Arr() {
		var roles []string
		roleMatch := strings.TrimSpace(role) == ""
		for _, ro := range u.Get("roles").Arr() {
			roles = append(roles, ro.Get("name").Str(ro.Get("shortname").Str("")))
			if !roleMatch && matches(role, ro.Get("shortname").Str(""), ro.Get("name").Str("")) {
				roleMatch = true
			}
		}
		if !roleMatch || !matches(query, u.Get("fullname").Str("")) {
			continue
		}
		out = append(out, m("userid", u.Get("id").Int(0), "name", u.Get("fullname").Str(""), "roles", roles,
			"last_course_access", s.time(u.Get("lastcourseaccess").Int(0))))
	}
	return out, nil
}

func (s *Service) moduleIndex(courseID int64) (names, types map[int64]string, err error) {
	secs, err := s.loadContents(courseID, false)
	if err != nil {
		return nil, nil, err
	}
	names, types = map[int64]string{}, map[int64]string{}
	for _, sec := range secs.Arr() {
		for _, mod := range sec.Get("modules").Arr() {
			id := mod.Get("id").Int(0)
			names[id] = Inline(mod.Get("name").Str(""))
			types[id] = mod.Get("modname").Str("")
		}
	}
	return names, types, nil
}

func (s *Service) Completion(courseID int64) (*M, error) {
	names, _, err := s.moduleIndex(courseID)
	if err != nil {
		return nil, err
	}
	uid, err := s.userID()
	if err != nil {
		return nil, err
	}
	r, err := s.C.Call("core_completion_get_activities_completion_status", map[string]any{"courseid": courseID, "userid": uid})
	if err != nil {
		return nil, err
	}
	done, open := []*M{}, []*M{}
	for _, st := range r.Get("statuses").Arr() {
		if !st.Get("hascompletion").Bool(true) {
			continue
		}
		state := st.Get("state").Int(0) // 0 incomplete, 1 complete, 2 pass, 3 fail
		cmid := st.Get("cmid").Int(0)
		completed := ""
		if state == 1 || state == 2 {
			completed = s.time(st.Get("timecompleted").Int(0))
		}
		row := m("cmid", cmid, "name", names[cmid], "type", st.Get("modname").Str(""), "completed", completed, "failed", ptrTrue(state == 3))
		if state == 1 || state == 2 {
			done = append(done, row)
		} else {
			open = append(open, row)
		}
	}
	return m("courseid", courseID, "completed", fmt.Sprintf("%d/%d", len(done), len(done)+len(open)), "open", open, "done", done), nil
}

func (s *Service) WhatsNew(courseIDs []int64, days int64) ([]*M, error) {
	since := s.now() - days*86400
	cs, err := s.enrolled()
	if err != nil {
		return nil, err
	}
	current, err := s.currentCourseIDs()
	if err != nil {
		return nil, err
	}
	out := []*M{}
	for _, c := range cs.Arr() {
		id := c.Get("id").Int(0)
		wanted := current[id]
		if len(courseIDs) > 0 {
			wanted = containsID(courseIDs, id)
		}
		if !wanted {
			continue
		}
		short := Inline(c.Get("shortname").Str(""))
		r, err := s.C.Call("core_course_get_updates_since", map[string]any{"courseid": id, "since": since})
		if err != nil || r.Get("instances").Len() == 0 {
			continue
		}
		names, types, err := s.moduleIndex(id)
		if err != nil {
			continue
		}
		for _, inst := range r.Get("instances").Arr() {
			cmid := inst.Get("id").Int(0)
			var changes []string
			for _, u := range inst.Get("updates").Arr() {
				changes = append(changes, describeUpdate(u))
			}
			typ := types[cmid]
			act := names[cmid]
			if act == "" {
				act = inst.Get("contextlevel").Str("")
			}
			u := ""
			if typ != "" {
				u = fmt.Sprintf("%s/mod/%s/view.php?id=%d", s.C.Base, typ, cmid)
			}
			out = append(out, m("course", short, "courseid", id, "cmid", cmid, "activity", act, "type", typ, "changes", changes, "url", u))
		}
	}
	return out, nil
}

func describeUpdate(u J) string {
	n := u.Get("itemids").Len()
	switch name := u.Get("name").Str(""); name {
	case "discussions", "posts":
		return fmt.Sprintf("%d neue Diskussionen/Beiträge", n)
	case "contentfiles":
		return fmt.Sprintf("%d neue/geänderte Datei(en)", n)
	case "introfiles":
		return "Dateien in der Beschreibung geändert"
	case "configuration":
		return "Beschreibung/Einstellungen geändert"
	case "submissions":
		return fmt.Sprintf("%d Abgabe(n) geändert", n)
	case "grades", "gradeitems", "outcomes":
		return "Bewertung geändert"
	case "completion":
		return "Abschlussstatus geändert"
	case "attempts":
		return fmt.Sprintf("%d Versuch(e)", n)
	case "entries":
		return fmt.Sprintf("%d neue Einträge", n)
	case "answers", "responses":
		return fmt.Sprintf("%d Antwort(en)", n)
	default:
		if name == "" {
			name = "?"
		}
		if n > 0 {
			return fmt.Sprintf("%s (%d)", name, n)
		}
		return name
	}
}

// ── quizzes & choices ───────────────────────────────────────────────────────

func (s *Service) Quizzes(courseID *int64, onlyOpen bool) ([]*M, error) {
	now := s.now()
	var p map[string]any
	if courseID != nil {
		p = map[string]any{"courseids": []int64{*courseID}}
	}
	r, err := s.C.Call("mod_quiz_get_quizzes_by_courses", p)
	if err != nil {
		return nil, err
	}
	current := map[int64]bool{}
	if onlyOpen {
		if current, err = s.currentCourseIDs(); err != nil {
			return nil, err
		}
	}
	shortnames := map[int64]string{}
	if courseID == nil {
		if cs, err := s.enrolled(); err == nil {
			for _, c := range cs.Arr() {
				shortnames[c.Get("id").Int(0)] = Inline(c.Get("shortname").Str(""))
			}
		}
	}
	out := []*M{}
	for _, q := range r.Get("quizzes").Arr() {
		closeT := q.Get("timeclose").Int(0)
		open := closeT >= now
		if closeT == 0 {
			open = current[q.Get("course").Int(0)]
		}
		if onlyOpen && !open {
			continue
		}
		maxGrade, sumGrades := q.Get("grade").Float(0), q.Get("sumgrades").Float(0)
		allowed := q.Get("attempts").Int(0)
		limit := q.Get("timelimit").Int(0)
		tl := ""
		if limit > 0 {
			if limit%60 == 0 {
				tl = fmt.Sprintf("%d min", limit/60)
			} else {
				tl = fmt.Sprintf("%d s", limit)
			}
		}
		var attemptsAllowed any = allowed
		if allowed == 0 {
			attemptsAllowed = "unbegrenzt"
		}
		mg := ""
		if maxGrade > 0 {
			mg = num(maxGrade)
		}
		cm := q.Get("coursemodule").Int(0)
		row := m("quizid", q.Get("id").Int(0), "cmid", cm, "course", shortnames[q.Get("course").Int(0)],
			"courseid", q.Get("course").Int(0), "name", Inline(q.Get("name").Str("")), "opens", s.time(q.Get("timeopen").Int(0)),
			"closes", s.time(closeT), "time_limit", tl, "attempts_allowed", attemptsAllowed, "max_grade", mg,
			"url", fmt.Sprintf("%s/mod/quiz/view.php?id=%d", s.C.Base, cm))
		if courseID != nil {
			quizID := q.Get("id").Int(0)
			ar, err := s.C.Call("mod_quiz_get_user_attempts", map[string]any{"quizid": quizID, "status": "all"})
			if err != nil {
				return nil, err
			}
			atts := []*M{}
			for _, a := range ar.Get("attempts").Arr() {
				g := ""
				if a.Get("sumgrades").IsNumber() && sumGrades > 0 {
					g = num(a.Get("sumgrades").Float(0)/sumGrades*maxGrade) + " / " + num(maxGrade)
				}
				atts = append(atts, m("attemptid", a.Get("id").Int(0), "attempt", a.Get("attempt").Int(0), "state", a.Get("state").Str(""),
					"started", s.time(a.Get("timestart").Int(0)), "finished", s.time(a.Get("timefinish").Int(0)), "grade", g))
			}
			row.Put("my_attempts", atts)
			best, err := s.C.Call("mod_quiz_get_user_best_grade", map[string]any{"quizid": quizID})
			if err == nil && best.Get("hasgrade").Bool(false) {
				row.Put("best_grade", num(best.Get("grade").Float(0))+" / "+num(maxGrade))
			}
		}
		out = append(out, row)
	}
	return out, nil
}

func (s *Service) QuizReview(attemptID int64) (*M, error) {
	r, err := s.C.Call("mod_quiz_get_attempt_review", map[string]any{"attemptid": attemptID, "page": -1})
	if err != nil {
		return nil, err
	}
	qs := []*M{}
	for _, q := range r.Get("questions").Arr() {
		mark := strings.TrimSpace(q.Get("mark").Str(""))
		if mark != "" {
			mark += " / " + num(q.Get("maxmark").Float(0))
		}
		qs = append(qs, m("slot", q.Get("slot").Int(0), "type", q.Get("type").Str(""), "status", Inline(q.Get("status").Str("")),
			"mark", mark, "text", Truncate(Blocks(q.Get("html").Str("")), 4000)))
	}
	feedback := ""
	for _, d := range r.Get("additionaldata").Arr() {
		if d.Get("id").Str("") == "feedback" {
			feedback = Blocks(d.Get("content").Str(""))
		}
	}
	return m("attemptid", attemptID, "state", r.Get("attempt").Get("state").Str(""), "grade", r.Get("grade").Str(""),
		"feedback", feedback, "questions", qs), nil
}

func (s *Service) Choices(courseID int64) ([]*M, error) {
	now := s.now()
	r, err := s.C.Call("mod_choice_get_choices_by_courses", map[string]any{"courseids": []int64{courseID}})
	if err != nil {
		return nil, err
	}
	out := []*M{}
	for _, c := range r.Get("choices").Arr() {
		from, until := c.Get("timeopen").Int(0), c.Get("timeclose").Int(0)
		opts, err := s.C.Call("mod_choice_get_choice_options", map[string]any{"choiceid": c.Get("id").Int(0)})
		if err != nil {
			return nil, err
		}
		options := []*M{}
		var mine []string
		for _, o := range opts.Get("options").Arr() {
			max, count := o.Get("maxanswers").Int(0), o.Get("countanswers").Int(0)
			text := Inline(o.Get("text").Str(""))
			if o.Get("checked").Bool(false) {
				mine = append(mine, text)
			}
			taken := fmt.Sprint(count)
			if max > 0 {
				taken = fmt.Sprintf("%d/%d", count, max)
			}
			options = append(options, m("optionid", o.Get("id").Int(0), "text", text, "taken", taken,
				"selected", ptrTrue(o.Get("checked").Bool(false)), "disabled", ptrTrue(o.Get("disabled").Bool(false))))
		}
		out = append(out, m("kind", "choice", "choiceid", c.Get("id").Int(0), "cmid", c.Get("coursemodule").Int(0),
			"name", Inline(c.Get("name").Str("")), "intro", Truncate(Blocks(c.Get("intro").Str("")), 2000),
			"opens", s.time(from), "closes", s.time(until), "open", from <= now && (until == 0 || until > now),
			"multiple", ptrTrue(c.Get("allowmultiple").Bool(false)), "can_change_answer", c.Get("allowupdate").Bool(false),
			"my_answer", mine, "options", options))
	}
	out = append(out, s.choiceGroups(courseID)...)
	return out, nil
}

// choiceGroups covers mod_choicegroup (best effort; its API differs between versions).
func (s *Service) choiceGroups(courseID int64) []*M {
	out := []*M{}
	secs, err := s.loadContents(courseID, false)
	if err != nil {
		return out
	}
	uid, _ := s.userID()
	for _, sec := range secs.Arr() {
		for _, mod := range sec.Get("modules").Arr() {
			if mod.Get("modname").Str("") != "choicegroup" || !mod.Get("uservisible").Bool(true) {
				continue
			}
			row := m("kind", "choicegroup", "choicegroupid", mod.Get("instance").Int(0), "cmid", mod.Get("id").Int(0),
				"name", Inline(mod.Get("name").Str("")), "url", mod.Get("url").Str(""))
			r, err := s.C.Call("mod_choicegroup_get_choicegroup_options", map[string]any{"choicegroupid": mod.Get("instance").Int(0), "userid": uid})
			if err != nil {
				row.Put("options_error", err.Error())
			} else {
				options := []*M{}
				for _, o := range r.Get("options").Arr() {
					options = append(options, m("optionid", o.Get("id").Int(0), "text", Inline(o.Get("text").Str(o.Get("name").Str(""))),
						"taken", o.Get("countanswers").Str(""), "selected", ptrTrue(o.Get("checked").Bool(false)),
						"disabled", ptrTrue(o.Get("disabled").Bool(false))))
				}
				row.Put("options", options)
			}
			out = append(out, row)
		}
	}
	return out
}

// ── write actions (preview unless confirm) ──────────────────────────────────

func preview(action string, kv ...any) *M {
	p := m("mode", "preview", "action", action)
	for i := 0; i+1 < len(kv); i += 2 {
		p.Put(kv[i].(string), kv[i+1])
	}
	p.Put("next", "Nichts wurde gesendet. Nach Bestätigung durch den Nutzer mit confirm=true erneut aufrufen.")
	return p
}

func requireText(s, name string) error {
	if strings.TrimSpace(s) == "" {
		return argErr("%s is required", name)
	}
	return nil
}

func toHTML(plain string) string {
	esc := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(plain)
	if strings.Contains(esc, "\n") {
		return "<p>" + strings.ReplaceAll(esc, "\n", "<br>") + "</p>"
	}
	return esc
}

func (s *Service) MarkNotificationsRead() (*M, error) {
	uid, err := s.userID()
	if err != nil {
		return nil, err
	}
	if _, err := s.C.CallWrite("core_message_mark_all_notifications_as_read", map[string]any{"useridto": uid}); err != nil {
		return nil, err
	}
	return m("marked_read", true), nil
}

// MarkMessagesRead marks every unread conversation (of the newest 50) as
// read; without confirm it only says which ones that would be.
func (s *Service) MarkMessagesRead(confirm bool) (*M, error) {
	uid, err := s.userID()
	if err != nil {
		return nil, err
	}
	convs, err := s.Conversations(50)
	if err != nil {
		return nil, err
	}
	var ids []int64
	var names []string
	for _, c := range convs {
		if c.Get("unread") == nil {
			continue
		}
		id, _ := c.Get("conversationid").(int64)
		name, _ := c.Get("name").(string)
		ids, names = append(ids, id), append(names, name)
	}
	if !confirm {
		return m("action", "Alle Moodle-Nachrichten als gelesen markieren", "conversations", len(ids), "names", names), nil
	}
	for _, id := range ids {
		if _, err := s.C.CallWrite("core_message_mark_all_conversation_messages_as_read", map[string]any{"userid": uid, "conversationid": id}); err != nil {
			return nil, err
		}
	}
	return m("marked_read", len(ids)), nil
}

func (s *Service) ForumReply(postID int64, subject, message string, confirm bool) (*M, error) {
	if err := requireText(message, "message"); err != nil {
		return nil, err
	}
	r, err := s.C.Call("mod_forum_get_discussion_post", map[string]any{"postid": postID})
	if err != nil {
		return nil, err
	}
	parent := r.Get("post")
	ps := Inline(parent.Get("subject").Str(""))
	subj := strings.TrimSpace(subject)
	if subj == "" {
		subj = ps
		if !strings.HasPrefix(ps, "Re:") {
			subj = "Re: " + ps
		}
	}
	if !confirm {
		return preview("Antwort im Forum auf Beitrag von "+parent.Get("author").Get("fullname").Str("?"), "subject", subj, "message", message), nil
	}
	w, err := s.C.CallWrite("mod_forum_add_discussion_post", map[string]any{"postid": postID, "subject": subj, "message": toHTML(message)})
	if err != nil {
		return nil, err
	}
	return m("mode", "sent", "postid", w.Get("postid").Int(0)), nil
}

func (s *Service) ForumPost(forumID int64, subject, message string, confirm bool) (*M, error) {
	if err := requireText(subject, "subject"); err != nil {
		return nil, err
	}
	if err := requireText(message, "message"); err != nil {
		return nil, err
	}
	if !confirm {
		r, err := s.C.Call("mod_forum_get_forums_by_courses", nil)
		if err != nil {
			return nil, err
		}
		forum := ""
		for _, f := range r.Arr() {
			if f.Get("id").Int(0) == forumID {
				forum = Inline(f.Get("name").Str(""))
			}
		}
		if forum == "" {
			return nil, &Error{Code: "notfound", Msg: fmt.Sprintf("Forum %d not found", forumID)}
		}
		return preview("Neue Diskussion im Forum '"+forum+"' (für alle Kursteilnehmer sichtbar)", "subject", subject, "message", message), nil
	}
	w, err := s.C.CallWrite("mod_forum_add_discussion", map[string]any{"forumid": forumID, "subject": subject, "message": toHTML(message)})
	if err != nil {
		return nil, err
	}
	return m("mode", "sent", "discussionid", w.Get("discussionid").Int(0)), nil
}

func (s *Service) SendMessage(conversationID, toUserID *int64, text string, confirm bool) (*M, error) {
	if err := requireText(text, "text"); err != nil {
		return nil, err
	}
	if (conversationID == nil) == (toUserID == nil) {
		return nil, argErr("exactly one of conversationid or userid is required")
	}
	if !confirm {
		to := ""
		if conversationID != nil {
			to = fmt.Sprintf("Unterhaltung %d", *conversationID)
		} else {
			to = fmt.Sprintf("Nutzer %d", *toUserID)
			if u, err := s.C.Call("core_user_get_users_by_field", map[string]any{"field": "id", "values": []int64{*toUserID}}); err == nil {
				to = u.Idx(0).Get("fullname").Str(to)
			}
		}
		return preview("Moodle-Nachricht an "+to, "text", text), nil
	}
	if conversationID != nil {
		_, err := s.C.CallWrite("core_message_send_messages_to_conversation", map[string]any{"conversationid": *conversationID,
			"messages": []map[string]any{{"text": text, "textformat": 2}}})
		if err != nil {
			return nil, err
		}
	} else {
		r, err := s.C.CallWrite("core_message_send_instant_messages", map[string]any{
			"messages": []map[string]any{{"touserid": *toUserID, "text": text, "textformat": 2}}})
		if err != nil {
			return nil, err
		}
		if msg := r.Idx(0).Get("errormessage").Str(""); r.Idx(0).Get("msgid").Int(0) < 0 || strings.TrimSpace(msg) != "" {
			return nil, &Error{Code: "send_failed", Msg: msg}
		}
	}
	return m("mode", "sent"), nil
}

func (s *Service) ChoiceSubmit(choiceID int64, optionIDs []int64, confirm bool) (*M, error) {
	if len(optionIDs) == 0 {
		return nil, argErr("optionids is required")
	}
	r, err := s.C.Call("mod_choice_get_choices_by_courses", nil)
	if err != nil {
		return nil, err
	}
	var choice *J
	for _, c := range r.Get("choices").Arr() {
		if c.Get("id").Int(0) == choiceID {
			c := c
			choice = &c
		}
	}
	if choice == nil {
		return nil, &Error{Code: "notfound", Msg: fmt.Sprintf("Choice %d not found", choiceID)}
	}
	if len(optionIDs) > 1 && !choice.Get("allowmultiple").Bool(false) {
		return nil, argErr("this choice allows only one option")
	}
	or, err := s.C.Call("mod_choice_get_choice_options", map[string]any{"choiceid": choiceID})
	if err != nil {
		return nil, err
	}
	options := map[int64]J{}
	var order []int64
	var previous, selection []string
	for _, o := range or.Get("options").Arr() {
		id := o.Get("id").Int(0)
		options[id] = o
		order = append(order, id)
		if o.Get("checked").Bool(false) {
			previous = append(previous, Inline(o.Get("text").Str("")))
		}
	}
	for _, id := range optionIDs {
		o, ok := options[id]
		if !ok {
			return nil, argErr("option %d does not exist; valid: %v", id, order)
		}
		if o.Get("disabled").Bool(false) {
			return nil, argErr("option %d is not selectable (full or closed)", id)
		}
		selection = append(selection, Inline(o.Get("text").Str("")))
	}
	if !confirm {
		p := preview("Abstimmung '"+Inline(choice.Get("name").Str(""))+"'", "selection", selection)
		p.Put("replaces", previous)
		return p, nil
	}
	if _, err := s.C.CallWrite("mod_choice_submit_choice_response", map[string]any{"choiceid": choiceID, "responses": optionIDs}); err != nil {
		return nil, err
	}
	return m("mode", "sent", "selection", selection), nil
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, p[2:])
		}
	}
	return p
}

func (s *Service) AssignmentSubmit(assignID int64, files []string, onlineText string, submitForGrading, confirm bool) (*M, error) {
	var paths []string
	for _, f := range files {
		p := expandHome(f)
		if st, err := os.Stat(p); err != nil || !st.Mode().IsRegular() {
			return nil, argErr("file not found: %s", f)
		}
		paths = append(paths, p)
	}
	hasText := strings.TrimSpace(onlineText) != ""
	if len(paths) == 0 && !hasText && !submitForGrading {
		return nil, argErr("nothing to submit: give files, online_text or submit_for_grading=true")
	}
	a, err := s.Assignment(assignID)
	if err != nil {
		return nil, err
	}
	status, _ := a.Get("submission").(*M)
	if status == nil {
		status = m()
	}
	if !confirm {
		info := []*M{}
		for _, p := range paths {
			st, _ := os.Stat(p)
			info = append(info, m("name", filepath.Base(p), "size", humanSize(st.Size())))
		}
		var warnings []string
		if existing, ok := status.Get("files").([]string); ok && len(paths) > 0 && len(existing) > 0 {
			warnings = append(warnings, fmt.Sprintf("Die bisher abgegebenen Dateien werden durch die neuen ersetzt: %v", existing))
		}
		if submitForGrading {
			warnings = append(warnings, "Nach dem Einreichen zur Bewertung ist die Abgabe meist nicht mehr änderbar.")
		}
		var chars any
		if hasText {
			chars = utf8.RuneCountInString(onlineText)
		}
		p := preview(fmt.Sprintf("Abgabe für '%v' (%v)", a.Get("name"), a.Get("course")), "assignment", a.Get("name"), "due", a.Get("due"),
			"current_status", status.Get("status"), "files", info, "online_text_chars", chars, "submit_for_grading", submitForGrading)
		p.Put("warning", strings.Join(warnings, " "))
		return p, nil
	}
	plugindata := map[string]any{}
	if len(paths) > 0 {
		item, err := s.C.UploadDraft(paths)
		if err != nil {
			return nil, err
		}
		plugindata["files_filemanager"] = item
	}
	if hasText {
		d, err := s.C.Call("core_files_get_unused_draft_itemid", nil)
		if err != nil {
			return nil, err
		}
		plugindata["onlinetext_editor"] = map[string]any{"text": toHTML(onlineText), "format": 1, "itemid": d.Get("itemid").Int(0)}
	}
	var warnings []string
	if len(plugindata) > 0 {
		w, err := s.C.CallWrite("mod_assign_save_submission", map[string]any{"assignmentid": assignID, "plugindata": plugindata})
		if err != nil {
			return nil, err
		}
		for _, x := range w.Arr() {
			warnings = append(warnings, x.Get("message").Str(fmt.Sprint(x.Raw())))
		}
		if len(warnings) > 0 {
			return m("mode", "failed", "warnings", warnings), nil
		}
	}
	if submitForGrading {
		w, err := s.C.CallWrite("mod_assign_submit_for_grading", map[string]any{"assignmentid": assignID, "acceptsubmissionstatement": 1})
		if err != nil {
			return nil, err
		}
		for _, x := range w.Arr() {
			warnings = append(warnings, x.Get("message").Str(fmt.Sprint(x.Raw())))
		}
		if len(warnings) > 0 {
			return m("mode", "saved_not_submitted", "warnings", warnings), nil
		}
	}
	result := "saved_as_draft"
	if submitForGrading {
		result = "submitted"
	}
	return m("mode", "sent", "result", result, "uploaded", baseNames(paths)), nil
}

// ── files & raw ─────────────────────────────────────────────────────────────

func (s *Service) Read(fileURL string, maxChars int) (string, error) {
	f, err := s.C.Fetch(fileURL)
	if err != nil {
		return "", err
	}
	typ := strings.ToLower(f.ContentType)
	var text string
	switch {
	case strings.HasPrefix(typ, "text/html") || strings.HasPrefix(typ, "application/xhtml"):
		text = Blocks(string(f.Body))
	case strings.HasPrefix(typ, "text/") || strings.Contains(typ, "json") || strings.Contains(typ, "xml"):
		text = string(f.Body)
	case strings.HasPrefix(typ, "application/pdf"):
		if text, err = PDFText(f.Body); err != nil {
			return "", err
		}
	default:
		return "", &Error{Code: "binary", Msg: fmt.Sprintf("'%s' is %s – use moodle_download and open the saved file instead", f.FileName, typ)}
	}
	return Truncate(text, maxChars), nil
}

func (s *Service) Download(fileURL, dir string) (*M, error) {
	p, n, err := s.C.Download(fileURL, expandHome(dir))
	if err != nil {
		return nil, err
	}
	abs, _ := filepath.Abs(p)
	return m("path", abs, "size", humanSize(int64(n))), nil
}

var (
	readOnlyRe  = regexp.MustCompile(`^(get|search|list|check|count)(_|$)`)
	sensitiveRe = regexp.MustCompile(`autologin|token|_key|password`)
)

// IsReadOnly classifies a web service function by Moodle's naming convention
// (component_area_verb…); view_* functions mark completion and are writes.
func IsReadOnly(wsfunction string) bool {
	f := strings.ToLower(wsfunction)
	parts := strings.SplitN(f, "_", 3)
	if len(parts) < 3 || sensitiveRe.MatchString(f) {
		return false
	}
	return readOnlyRe.MatchString(parts[2])
}

var ErrConfirmRequired = errors.New("confirm required")

func (s *Service) RawCall(wsfunction string, params map[string]any, confirm bool) (J, error) {
	if IsReadOnly(wsfunction) {
		return s.C.Call(wsfunction, params)
	}
	if !confirm {
		return J{}, &Error{Code: "confirm_required", Msg: wsfunction + " may change data in Moodle (submit, post, mark as viewed …). Ask the user, then repeat with confirm=true."}
	}
	return s.C.CallWrite(wsfunction, params)
}

// ── helpers ─────────────────────────────────────────────────────────────────

func humanSize(b int64) string {
	switch {
	case b < 0:
		return ""
	case b < 1024:
		return fmt.Sprintf("%d B", b)
	case b < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024))
}

func matches(query string, fields ...string) bool {
	if strings.TrimSpace(query) == "" {
		return true
	}
	hay := strings.ToLower(strings.Join(fields, " "))
	for _, t := range strings.Fields(strings.ToLower(query)) {
		if !strings.Contains(hay, t) {
			return false
		}
	}
	return true
}

func containsID(ids []int64, id int64) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}
