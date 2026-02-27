package tests

import (
	"context"
	"os"
	"testing"

	"github.com/jony/son-of-anthon/pkg/skills/architect"
	"github.com/jony/son-of-anthon/pkg/skills/atc"
	"github.com/jony/son-of-anthon/pkg/skills/coach"
)

// TestNextcloudATC ensures that ATC can hit the dynamic Tasks URL properly
func TestNextcloudATC(t *testing.T) {
	atcSkill := &atc.ATCSkill{}
	res := atcSkill.Execute(context.Background(), map[string]interface{}{"command": "list_nextcloud_tasks"})
	if res.IsError {
		t.Fatalf("ATC list_nextcloud_tasks failed: %s", res.ForLLM)
	}
	t.Logf("ATC Success:\n%s", res.ForLLM)
}

// TestNextcloudCoach ensures the WebDAV file extraction works via dynamic URL
func TestNextcloudCoach(t *testing.T) {
	os.Setenv("PERSONAL_OS_CONFIG", os.Getenv("HOME")+"/.picoclaw/config.json")
	coachSkill := coach.NewSkill()

	res := coachSkill.Execute(context.Background(), map[string]interface{}{"command": "generate_practice"})
	if res.IsError {
		t.Fatalf("Coach generate_practice failed: %s", res.ForLLM)
	}
	t.Logf("Coach Success:\n%s", res.ForLLM)
}

// TestNextcloudArchitect ensures CalDAV event sync hits both static and recurring endpoints
func TestNextcloudArchitect(t *testing.T) {
	archSkill := architect.NewSkill()
	res := archSkill.Execute(context.Background(), map[string]interface{}{"command": "sync_deadlines"})
	if res.IsError {
		t.Fatalf("Architect sync_deadlines failed: %s", res.ForLLM)
	}
	t.Logf("Architect Success:\n%s", res.ForLLM)
}
