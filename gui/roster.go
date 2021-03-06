package gui

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"log"
	"runtime"
	"sort"
	"strings"

	"github.com/coyim/coyim/config"
	"github.com/coyim/coyim/i18n"
	rosters "github.com/coyim/coyim/roster"
	"github.com/coyim/coyim/xmpp/jid"
	"github.com/coyim/gotk3adapter/gdki"
	"github.com/coyim/gotk3adapter/gtki"
)

type roster struct {
	widget gtki.ScrolledWindow
	model  gtki.TreeStore
	view   gtki.TreeView

	isCollapsed map[string]bool
	toCollapse  []gtki.TreePath

	ui *gtkUI
}

const (
	indexJid               = 0
	indexDisplayName       = 1
	indexAccountID         = 2
	indexColor             = 3
	indexBackgroundColor   = 4
	indexWeight            = 5
	indexParentJid         = 0
	indexParentDisplayName = 1
	indexTooltip           = 6
	indexStatusIcon        = 7
	indexRowType           = 8
)

func (u *gtkUI) newRoster() *roster {
	builder := newBuilder("Roster")

	r := &roster{
		isCollapsed: make(map[string]bool),

		ui: u,
	}

	builder.ConnectSignals(map[string]interface{}{
		"on_activate_buddy": r.onActivateRosterRow,
		"on_button_press":   r.onButtonPress,
	})

	obj := builder.getObj("roster")
	r.widget = obj.(gtki.ScrolledWindow)

	obj = builder.getObj("roster-tree")
	r.view = obj.(gtki.TreeView)
	r.view.SetEnableSearch(true)
	r.view.SetSearchEqualSubstringMatch()

	obj = builder.getObj("roster-model")
	r.model = obj.(gtki.TreeStore)

	//r.model needs to be kept beyond the lifespan of the builder.
	r.model.Ref()
	runtime.SetFinalizer(r, func(ros interface{}) {
		ros.(*roster).model.Unref()
		ros.(*roster).model = nil
	})

	return r
}

func getFromModelIter(m gtki.TreeStore, iter gtki.TreeIter, index int) string {
	val, _ := m.GetValue(iter, index)
	v, _ := val.GetString()
	return v
}

func (r *roster) getAccountAndJidFromEvent(bt gdki.EventButton) (j jid.WithoutResource, account *account, rowType string, ok bool) {
	x := bt.X()
	y := bt.Y()
	path := g.gtk.TreePathNew()
	found := r.view.GetPathAtPos(int(x), int(y), path, nil, nil, nil)
	if !found {
		return nil, nil, "", false
	}
	iter, err := r.model.GetIter(path)
	if err != nil {
		return nil, nil, "", false
	}
	j = jid.NR(getFromModelIter(r.model, iter, indexJid))
	accountID := getFromModelIter(r.model, iter, indexAccountID)
	rowType = getFromModelIter(r.model, iter, indexRowType)
	account, ok = r.ui.accountManager.getAccountByID(accountID)
	return j, account, rowType, ok
}

func sortedGroupNames(groups map[string]bool) []string {
	sortedNames := make([]string, 0, len(groups))
	for k := range groups {
		sortedNames = append(sortedNames, k)
	}

	sort.Strings(sortedNames)

	return sortedNames
}

func (r *roster) allGroupNames() []string {
	groups := map[string]bool{}
	for _, contacts := range r.ui.accountManager.getAllContacts() {
		for name := range contacts.GetGroupNames() {
			if groups[name] {
				continue
			}

			groups[name] = true
		}
	}

	return sortedGroupNames(groups)
}

func (r *roster) getGroupNamesFor(a *account) []string {
	groups := map[string]bool{}
	contacts := r.ui.accountManager.getAllContacts()[a]
	for name := range contacts.GetGroupNames() {
		if groups[name] {
			continue
		}

		groups[name] = true
	}

	return sortedGroupNames(groups)
}

func (r *roster) updatePeer(acc *account, jid jid.WithoutResource, nickname string, groups []string, updateRequireEncryption, requireEncryption bool) error {
	peer, ok := r.ui.getPeer(acc, jid)
	if !ok {
		return fmt.Errorf("Could not find peer %s", jid)
	}

	// This updates what is displayed in the roster
	peer.Nickname = nickname
	peer.SetGroups(groups)

	// NOTE: This requires the account to be connected in order to rename peers,
	// which should not be the case. This is one example of why gui.account should
	// own the account config -  and not the session.
	conf := acc.session.GetConfig()
	conf.SavePeerDetails(jid.String(), nickname, groups)
	if updateRequireEncryption {
		conf.UpdateEncryptionRequired(jid.String(), requireEncryption)
	}

	r.ui.SaveConfig()
	doInUIThread(r.redraw)

	return nil
}

func (r *roster) renamePeer(acc *account, peer jid.WithoutResource, nickname string) {
	p, ok := r.ui.getPeer(acc, peer)
	if !ok {
		return
	}

	// This updates what is displayed in the roster
	p.Nickname = nickname

	// This saves the nickname to the config file
	// NOTE: This requires the account to be connected in order to rename peers,
	// which should not be the case. This is one example of why gui.account should
	// own the account config -  and not the session.
	//acc.session.GetConfig().RenamePeer(jid, nickname)

	doInUIThread(r.redraw)
	r.ui.SaveConfig()
}

func toArray(groupList gtki.ListStore) []string {
	groups := []string{}

	iter, ok := groupList.GetIterFirst()
	for ok {
		gValue, _ := groupList.GetValue(iter, 0)
		if group, err := gValue.GetString(); err == nil {
			groups = append(groups, group)
		}

		ok = groupList.IterNext(iter)
	}

	return groups
}

func (r *roster) setSensitive(menuItem gtki.MenuItem, account *account, peer jid.WithoutResource) {
	p, ok := r.ui.getPeer(account, peer)
	if !ok {
		return
	}

	menuItem.SetSensitive(p.HasResources())
}

func (r *roster) createAccountPeerPopup(jid jid.WithoutResource, account *account, bt gdki.EventButton) {
	builder := newBuilder("ContactPopupMenu")
	mn := builder.getObj("contactMenu").(gtki.Menu)

	resourcesMenuItem := builder.getObj("resourcesMenuItem").(gtki.MenuItem)
	r.appendResourcesAsMenuItems(jid, account, resourcesMenuItem)

	sendFileMenuItem := builder.getObj("sendFileMenuItem").(gtki.MenuItem)
	r.setSensitive(sendFileMenuItem, account, jid)

	sendDirMenuItem := builder.getObj("sendDirectoryMenuItem").(gtki.MenuItem)
	r.setSensitive(sendDirMenuItem, account, jid)

	builder.ConnectSignals(map[string]interface{}{
		"on_remove_contact": func() {
			account.session.RemoveContact(jid.String())
			r.ui.removePeer(account, jid)
			r.redraw()
		},
		"on_edit_contact": func() {
			doInUIThread(func() { r.openEditContactDialog(jid, account) })
		},
		"on_allow_contact_to_see_status": func() {
			account.session.ApprovePresenceSubscription(jid, "" /* generate id */)
		},
		"on_forbid_contact_to_see_status": func() {
			account.session.DenyPresenceSubscription(jid, "" /* generate id */)
		},
		"on_ask_contact_to_see_status": func() {
			account.session.RequestPresenceSubscription(jid, "")
		},
		"on_dump_info": func() {
			r.ui.accountManager.debugPeersFor(account)
		},
		"on_send_file_to_contact": func() {
			if peer, ok := r.ui.getPeer(account, jid); ok {
				// TODO: It's a real problem to start file transfer if we don't have a resource, so we should ensure that here
				// (Because disco#info will not actually return results from the CLIENT unless a resource is prefixed...
				doInUIThread(func() {
					account.sendFileTo(jid.WithResource(peer.ResourceToUse()), r.ui)
				})
			}
		},
		"on_send_directory_to_contact": func() {
			if peer, ok := r.ui.getPeer(account, jid); ok {
				// TODO: It's a real problem to start file transfer if we don't have a resource, so we should ensure that here
				// (Because disco#info will not actually return results from the CLIENT unless a resource is prefixed...
				doInUIThread(func() {
					account.sendDirectoryTo(jid.WithResource(peer.ResourceToUse()), r.ui)
				})
			}
		},
	})

	mn.ShowAll()
	mn.PopupAtPointer(bt)
}

func (r *roster) appendResourcesAsMenuItems(jid jid.WithoutResource, account *account, menuItem gtki.MenuItem) {
	peer, ok := r.ui.getPeer(account, jid)
	if !ok {
		return
	}

	hasResources := peer.HasResources()
	menuItem.SetSensitive(hasResources)

	if !hasResources {
		return
	}

	innerMenu, _ := g.gtk.MenuNew()
	for _, resource := range peer.Resources() {
		item, _ := g.gtk.CheckMenuItemNewWithMnemonic(string(resource))
		rs := resource
		item.Connect("activate",
			func() {
				doInUIThread(func() {
					r.ui.openTargetedConversationView(account, jid.WithResource(rs), true)
				})
			})
		innerMenu.Append(item)
	}

	menuItem.SetSubmenu(innerMenu)
}

func (r *roster) createAccountPopup(account *account, bt gdki.EventButton) {
	mn := account.createSubmenu()
	if *config.DebugFlag {
		mn.Append(account.createSeparatorItem())
		mn.Append(account.createDumpInfoItem(r))
		mn.Append(account.createXMLConsoleItem(r.ui.window))
	}
	mn.ShowAll()
	mn.PopupAtPointer(bt)
}

func (r *roster) onButtonPress(view gtki.TreeView, ev gdki.Event) bool {
	bt := g.gdk.EventButtonFrom(ev)
	if bt.Button() == 0x03 {
		jid, account, rowType, ok := r.getAccountAndJidFromEvent(bt)
		if ok {
			switch rowType {
			case "peer":
				r.createAccountPeerPopup(jid.NoResource(), account, bt)
			case "account":
				r.createAccountPopup(account, bt)
			}
		}
	}

	return false
}

func collapseTransform(s string) string {
	res := sha256.Sum256([]byte(s))
	return hex.EncodeToString(res[:])
}

func (r *roster) restoreCollapseStatus() {
	pieces := strings.Split(r.ui.settings.GetCollapsed(), ":")
	for _, p := range pieces {
		if p != "" {
			r.isCollapsed[p] = true
		}
	}
}

func (r *roster) saveCollapseStatus() {
	var vals []string
	for e, v := range r.isCollapsed {
		if v {
			vals = append(vals, e)
		}
	}
	r.ui.settings.SetCollapsed(strings.Join(vals, ":"))
}

func (r *roster) activateAccountRow(jid string) {
	ix := collapseTransform(jid)
	r.isCollapsed[ix] = !r.isCollapsed[ix]
	r.saveCollapseStatus()
	r.redraw()
}

func (r *roster) onActivateRosterRow(v gtki.TreeView, path gtki.TreePath) {
	iter, err := r.model.GetIter(path)
	if err != nil {
		return
	}

	peer := getFromModelIter(r.model, iter, indexJid)
	rowType := getFromModelIter(r.model, iter, indexRowType)

	switch rowType {
	case "peer":
		selection, err := v.GetSelection()
		if err != nil {
			return
		}

		defer selection.UnselectPath(path)
		accountID := getFromModelIter(r.model, iter, indexAccountID)
		account, ok := r.ui.accountManager.getAccountByID(accountID)
		if !ok {
			return
		}
		r.ui.openConversationView(account, jid.NR(peer), true)
	case "account":
		r.activateAccountRow(peer)
	default:
		panic(fmt.Sprintf("unknown roster row type: %s", rowType))
	}
}

func (r *roster) displayNameFor(account *account, from jid.WithoutResource) string {
	p, ok := r.ui.getPeer(account, from)
	if !ok {
		return from.String()
	}

	return p.NameForPresentation()
}

func (r *roster) update(account *account, entries *rosters.List) {
	r.ui.accountManager.Lock()
	defer r.ui.accountManager.Unlock()

	r.ui.accountManager.setContacts(account, entries)
}

func isNominallyVisible(p *rosters.Peer, showWaiting bool) bool {
	return (p.Subscription != "none" && p.Subscription != "") || (showWaiting && (p.PendingSubscribeID != "" || p.Asked))
}

func shouldDisplay(p *rosters.Peer, showOffline, showWaiting bool) bool {
	return isNominallyVisible(p, showWaiting) && (showOffline || p.IsOnline() || p.Asked)
}

func isAway(p *rosters.Peer) bool {
	switch p.MainStatus() {
	case "dnd", "xa", "away":
		return true
	}
	return false
}

func isOnline(p *rosters.Peer) bool {
	return p.PendingSubscribeID == "" && p.IsOnline()
}

func decideStatusFor(p *rosters.Peer) string {
	if p.PendingSubscribeID != "" || p.Asked {
		return "unknown"
	}

	if !p.IsOnline() {
		return "offline"
	}

	switch p.MainStatus() {
	case "dnd":
		return "busy"
	case "xa":
		return "extended-away"
	case "away":
		return "away"
	}

	return "available"
}

func decideColorFor(cs colorSet, p *rosters.Peer) string {
	if !p.IsOnline() {
		return cs.rosterPeerOfflineForeground
	}
	return cs.rosterPeerOnlineForeground
}

func createGroupDisplayName(parentName string, counter *counter, isExpanded bool) string {
	name := parentName
	if !isExpanded {
		name = fmt.Sprintf("[%s]", name)
	}
	return fmt.Sprintf("%s (%d/%d)", name, counter.online, counter.total)
}

func createTooltipFor(item *rosters.Peer) string {
	pname := html.EscapeString(item.NameForPresentation())
	jid := html.EscapeString(item.Jid.String())
	if pname != jid {
		return fmt.Sprintf("%s (%s)", pname, jid)
	}
	return jid
}

func (r *roster) addItem(item *rosters.Peer, parentIter gtki.TreeIter, indent string) {
	cs := r.ui.currentColorSet()
	iter := r.model.Append(parentIter)
	potentialExtra := ""
	if item.Asked {
		potentialExtra = i18n.Local(" (waiting for approval)")
	}
	setAll(r.model, iter,
		item.Jid.String(),
		fmt.Sprintf("%s %s%s", indent, item.NameForPresentation(), potentialExtra),
		item.BelongsTo,
		decideColorFor(cs, item),
		cs.rosterPeerBackground,
		nil,
		createTooltipFor(item),
	)

	r.model.SetValue(iter, indexRowType, "peer")
	r.model.SetValue(iter, indexStatusIcon, statusIcons[decideStatusFor(item)].getPixbuf())
}

func (r *roster) redrawMerged() {
	showOffline := !r.ui.config.Display.ShowOnlyOnline
	showWaiting := !r.ui.config.Display.ShowOnlyConfirmed

	r.ui.accountManager.RLock()
	defer r.ui.accountManager.RUnlock()

	r.toCollapse = nil

	grp := rosters.TopLevelGroup()
	for account, contacts := range r.ui.accountManager.getAllContacts() {
		contacts.AddTo(grp, account.session.GroupDelimiter())
	}

	accountCounter := &counter{}
	r.displayGroup(grp, nil, accountCounter, showOffline, showWaiting, "")

	r.view.ExpandAll()
	for _, path := range r.toCollapse {
		r.view.CollapseRow(path)
	}
}

type counter struct {
	total  int
	online int
}

func (c *counter) inc(total, online bool) {
	if total {
		c.total++
	}
	if online {
		c.online++
	}
}

func (r *roster) sortedPeers(ps []*rosters.Peer) []*rosters.Peer {
	if r.ui.config.Display.SortByStatus {
		sort.Sort(byStatus(ps))
	} else {
		sort.Sort(byNameForPresentation(ps))
	}
	return ps
}

type byNameForPresentation []*rosters.Peer

func (s byNameForPresentation) Len() int { return len(s) }
func (s byNameForPresentation) Less(i, j int) bool {
	return s[i].NameForPresentation() < s[j].NameForPresentation()
}
func (s byNameForPresentation) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

func statusValueFor(s string) int {
	switch s {
	case "available":
		return 0
	case "away":
		return 1
	case "extended-away":
		return 2
	case "busy":
		return 3
	case "offline":
		return 4
	case "unknown":
		return 5
	}
	return -1
}

type byStatus []*rosters.Peer

func (s byStatus) Len() int { return len(s) }
func (s byStatus) Less(i, j int) bool {
	return statusValueFor(decideStatusFor(s[i])) < statusValueFor(decideStatusFor(s[j]))
}
func (s byStatus) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

func (r *roster) displayGroup(g *rosters.Group, parentIter gtki.TreeIter, accountCounter *counter, showOffline, showWaiting bool, accountName string) {
	pi := parentIter
	groupCounter := &counter{}
	groupID := accountName + "//" + g.FullGroupName()

	isEmpty := true
	for _, item := range g.UnsortedPeers() {
		if shouldDisplay(item, showOffline, showWaiting) {
			isEmpty = false
		}
	}

	if g.GroupName != "" && (!isEmpty || r.showEmptyGroups()) {
		pi = r.model.Append(parentIter)
		r.model.SetValue(pi, indexParentJid, groupID)
		r.model.SetValue(pi, indexRowType, "group")
		r.model.SetValue(pi, indexWeight, 500)
		r.model.SetValue(pi, indexBackgroundColor, r.ui.currentColorSet().rosterGroupBackground)
	}

	for _, item := range r.sortedPeers(g.UnsortedPeers()) {
		vs := isNominallyVisible(item, showWaiting)
		o := isOnline(item)
		accountCounter.inc(vs, vs && o)
		groupCounter.inc(vs, vs && o)

		if shouldDisplay(item, showOffline, showWaiting) {
			r.addItem(item, pi, "")
		}
	}

	for _, gr := range g.Groups() {
		r.displayGroup(gr, pi, accountCounter, showOffline, showWaiting, accountName)
	}

	if g.GroupName != "" && (!isEmpty || r.showEmptyGroups()) {
		parentPath, _ := r.model.GetPath(pi)
		shouldCollapse, ok := r.isCollapsed[collapseTransform(groupID)]
		isExpanded := true
		if ok && shouldCollapse {
			isExpanded = false
			r.toCollapse = append(r.toCollapse, parentPath)
		}

		r.model.SetValue(pi, indexParentDisplayName, createGroupDisplayName(g.FullGroupName(), groupCounter, isExpanded))
	}
}

func (r *roster) redrawSeparateAccount(account *account, contacts *rosters.List, showOffline, showWaiting bool) {
	cs := r.ui.currentColorSet()
	parentIter := r.model.Append(nil)

	accountCounter := &counter{}

	grp := contacts.Grouped(account.session.GroupDelimiter())
	parentName := account.session.GetConfig().Account
	r.displayGroup(grp, parentIter, accountCounter, showOffline, showWaiting, parentName)

	r.model.SetValue(parentIter, indexParentJid, parentName)
	r.model.SetValue(parentIter, indexAccountID, account.session.GetConfig().ID())
	r.model.SetValue(parentIter, indexRowType, "account")
	r.model.SetValue(parentIter, indexWeight, 700)

	bgcolor := cs.rosterAccountOnlineBackground
	if account.session.IsDisconnected() {
		bgcolor = cs.rosterAccountOfflineBackground
	}
	r.model.SetValue(parentIter, indexBackgroundColor, bgcolor)

	parentPath, _ := r.model.GetPath(parentIter)
	shouldCollapse, ok := r.isCollapsed[collapseTransform(parentName)]
	isExpanded := true
	if ok && shouldCollapse {
		isExpanded = false
		r.toCollapse = append(r.toCollapse, parentPath)
	}
	var stat string
	if account.session.IsDisconnected() {
		stat = "offline"
	} else if account.session.IsConnected() {
		stat = "available"
	} else {
		stat = "connecting"
	}

	r.model.SetValue(parentIter, indexStatusIcon, statusIcons[stat].getPixbuf())
	r.model.SetValue(parentIter, indexParentDisplayName, createGroupDisplayName(parentName, accountCounter, isExpanded))
}

func (r *roster) sortedAccounts() []*account {
	var as []*account
	for account := range r.ui.accountManager.getAllContacts() {
		if account == nil {
			log.Printf("adding an account that is nil...\n")
		}
		as = append(as, account)
	}
	//TODO sort by nickname if available
	sort.Sort(byAccountNameAlphabetic(as))
	return as
}

func (r *roster) showEmptyGroups() bool {
	return r.ui.settings.GetShowEmptyGroups()
}

func (r *roster) redrawSeparate() {
	showOffline := !r.ui.config.Display.ShowOnlyOnline
	showWaiting := !r.ui.config.Display.ShowOnlyConfirmed

	r.ui.accountManager.RLock()
	defer r.ui.accountManager.RUnlock()

	r.toCollapse = nil

	for _, account := range r.sortedAccounts() {
		r.redrawSeparateAccount(account, r.ui.accountManager.getContacts(account), showOffline, showWaiting)
	}

	r.view.ExpandAll()
	for _, path := range r.toCollapse {
		r.view.CollapseRow(path)
	}
}

const disconnectedPageIndex = 0
const spinnerPageIndex = 1
const rosterPageIndex = 2

func (r *roster) redraw() {
	//TODO: this should be behind a mutex
	r.model.Clear()

	if r.ui.shouldViewAccounts() {
		r.redrawSeparate()
	} else {
		r.redrawMerged()
	}
}

func setAll(v gtki.TreeStore, iter gtki.TreeIter, values ...interface{}) {
	for i, val := range values {
		if val != nil {
			v.SetValue(iter, i, val)
		}
	}
}
