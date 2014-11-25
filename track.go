package libkb

import (
	"fmt"
	"github.com/keybase/go-jsonw"
	"sync"
	"time"
)

// Can either be a RemoteProofChainLink or one of the identities
// listed in a tracking statement
type TrackIdComponent interface {
	ToIdString() string
	ToKeyValuePair() (string, string)
	GetProofState() int
}

type TrackSet map[string]TrackIdComponent

func (ts TrackSet) Add(t TrackIdComponent) {
	ts[t.ToIdString()] = t
}

func (a TrackSet) GetProofState(tic TrackIdComponent) int {
	ret := PROOF_STATE_NONE
	if obj := a[tic.ToIdString()]; obj != nil {
		ret = obj.GetProofState()
	}
	return ret
}

func (A TrackSet) Subtract(B TrackSet) (out []TrackIdComponent) {
	for _, c := range A {
		if !B.HasMember(c) {
			out = append(out, c)
		}
	}
	return
}

func (a TrackSet) HasMember(t TrackIdComponent) bool {
	_, found := a[t.ToIdString()]
	return found
}

func (a TrackSet) LenEq(b TrackSet) bool {
	return len(a) == len(b)
}

//=====================================================================

type TrackInstructions struct {
	Local  bool
	Remote bool
}

//=====================================================================

type TrackLookup struct {
	link  *TrackChainLink     // The original chain link that I signed
	set   TrackSet            // The total set of tracked identities
	ids   map[string][]string // A http -> [foo.com, boo.com] lookup
	mutex *sync.Mutex         // in case we're accessing in mutliple threads
}

func (l TrackLookup) GetProofState(tic TrackIdComponent) int {
	return l.set.GetProofState(tic)
}

func (tl *TrackLookup) ComputeKeyDiff(curr PgpFingerprint) TrackDiff {
	prev, err := tl.link.GetTrackedPgpFingerprint()
	if err != nil {
		return TrackDiffError{err}
	} else if prev.Eq(curr) {
		return TrackDiffNone{}
	} else {
		return TrackDiffClash{curr.ToQuads(), prev.ToQuads()}
	}
}

func (tl TrackLookup) IsRemote() bool {
	return tl.link.IsRemote()
}

type TrackDiff interface {
	BreaksTracking() bool
	ToDisplayString() string
	IsSameAsTracked() bool
}

type TrackDiffError struct {
	err error
}

func (t TrackDiffError) BreaksTracking() bool {
	return true
}
func (t TrackDiffError) ToDisplayString() string {
	return "error"
}
func (t TrackDiffError) IsSameAsTracked() bool {
	return false
}

type TrackDiffUpgraded struct {
	prev, curr string
}

func (t TrackDiffUpgraded) IsSameAsTracked() bool {
	return false
}

func (t TrackDiffUpgraded) BreaksTracking() bool {
	return false
}
func (t TrackDiffUpgraded) ToDisplayString() string {
	return "Upgraded from " + t.prev + " to " + t.curr
}
func (t TrackDiffUpgraded) GetPrev() string { return t.prev }
func (t TrackDiffUpgraded) GetCurr() string { return t.curr }

type TrackDiffNone struct{}

func (t TrackDiffNone) BreaksTracking() bool {
	return false
}
func (t TrackDiffNone) IsSameAsTracked() bool {
	return true
}

func (t TrackDiffNone) ToDisplayString() string {
	return "OK"
}

type TrackDiffNew struct{}

func (t TrackDiffNew) BreaksTracking() bool {
	return false
}
func (t TrackDiffNew) IsSameAsTracked() bool {
	return false
}

type TrackDiffClash struct {
	observed, expected string
}

func (t TrackDiffNew) ToDisplayString() string {
	return "new"
}

func (t TrackDiffClash) BreaksTracking() bool {
	return true
}

func (t TrackDiffClash) ToDisplayString() string {
	return "CHANGED from " + t.expected
}
func (t TrackDiffClash) IsSameAsTracked() bool {
	return false
}

type TrackDiffDeleted struct {
	idc TrackIdComponent
}

func (t TrackDiffDeleted) BreaksTracking() bool {
	return true
}
func (t TrackDiffDeleted) ToDisplayString() string {
	return "Deleted proof: " + t.idc.ToIdString()
}
func (t TrackDiffDeleted) IsSameAsTracked() bool {
	return false
}

type TrackDiffRemoteFail struct {
	observed int
}

func (t TrackDiffRemoteFail) BreaksTracking() bool {
	return true
}
func (t TrackDiffRemoteFail) ToDisplayString() string {
	return "remote failed"
}
func (t TrackDiffRemoteFail) IsSameAsTracked() bool {
	return false
}

type TrackDiffRemoteWorking struct {
	tracked int
}

func (t TrackDiffRemoteWorking) BreaksTracking() bool {
	return false
}
func (t TrackDiffRemoteWorking) ToDisplayString() string {
	return "working"
}
func (t TrackDiffRemoteWorking) IsSameAsTracked() bool {
	return false
}

type TrackDiffRemoteChanged struct {
	tracked, observed int
}

func (t TrackDiffRemoteChanged) BreaksTracking() bool {
	return false
}
func (t TrackDiffRemoteChanged) ToDisplayString() string {
	return "changed"
}
func (t TrackDiffRemoteChanged) IsSameAsTracked() bool {
	return false
}

func NewTrackLookup(link *TrackChainLink) *TrackLookup {
	sbs := link.ToServiceBlocks()
	set := make(TrackSet)
	ids := make(map[string][]string)
	for _, sb := range sbs {
		set.Add(sb)
		k, v := sb.ToKeyValuePair()
		list, found := ids[k]
		if !found {
			list = make([]string, 0, 1)
		}
		ids[k] = append(list, v)
	}
	ret := &TrackLookup{link: link, set: set, ids: ids, mutex: new(sync.Mutex)}
	return ret
}

func (l *TrackLookup) Lock() {
	l.mutex.Lock()
}

func (l *TrackLookup) Unlock() {
	l.mutex.Unlock()
}

func (e *TrackLookup) GetCTime() time.Time {
	return e.link.GetCTime()
}

//=====================================================================

type TrackEngine struct {
	TheirName    string
	Them         *User
	Me           *User
	Interactive  bool
	NoSelf       bool
	StrictProofs bool
	MeRequired   bool

	trackStatementBytes []byte
	trackStatement      *jsonw.Wrapper
	signingKey          *PgpKeyBundle
	sig                 string
	sigid               *SigId
}

func (e *TrackEngine) LoadThem() error {

	if e.Them == nil && len(e.TheirName) == 0 {
		return fmt.Errorf("No 'them' passed to TrackEngine")
	}
	if e.Them == nil {
		if u, err := LoadUser(LoadUserArg{
			Name:        e.TheirName,
			Self:        false,
			LoadSecrets: false,
			ForceReload: false,
		}); err != nil {
			return err
		} else {
			e.Them = u
		}
	}
	return nil
}

func (e *TrackEngine) LoadMe() error {
	if e.Me == nil {
		if me, err := LoadMe(LoadUserArg{LoadSecrets: true}); err != nil && e.MeRequired {
			return err
		} else {
			e.Me = me
		}
	}
	return nil
}

func (e *TrackEngine) Run() (err error) {

	if err = e.LoadThem(); err != nil {
		return
	} else if err = e.LoadMe(); err != nil {
		return
	} else if e.NoSelf && e.Me.Equal(*e.Them) {
		err = fmt.Errorf("Cannot track yourself")
	}

	var ti TrackInstructions
	ti, err = e.Them.Identify(IdentifyArg{
		Me: e.Me,
		Ui: G.UI.GetIdentifyTrackUI(e.Them, e.StrictProofs),
	})

	if err != nil {
		return
	}

	e.trackStatement, err = e.Me.TrackingProofFor(e.Them)
	if err != nil {
		return err
	}

	if e.trackStatementBytes, err = e.trackStatement.Marshal(); err != nil {
		return
	}

	G.Log.Debug("| Tracking statement: %s", string(e.trackStatementBytes))

	if ti.Remote {
		err = e.StoreRemoteTrack()
	} else if ti.Local {
		err = e.StoreLocalTrack()
	}
	return
}

func GetLocalTrack(i UID) (ret *TrackChainLink, err error) {
	uid_s := i.ToString()
	G.Log.Debug("+ GetLocalTrack(%s)", uid_s)
	defer G.Log.Debug("- GetLocalTrack(%s) -> %s", uid_s, ErrToOk(err))

	var obj *jsonw.Wrapper
	obj, err = G.LocalDb.Get(
		DbKey{Typ: DB_LOCAL_TRACK, Key: i.ToString()},
	)
	if err != nil {
		G.Log.Debug("| DB lookup failed")
		return
	}

	cl := &ChainLink{payloadJson: obj, unsigned: true}
	if err = cl.UnpackLocal(); err != nil {
		G.Log.Debug("| unpack failed -> %s", err.Error())
		return
	}
	base := GenericChainLink{cl}
	ret, err = ParseTrackChainLink(base)
	if ret != nil && err == nil {
		ret.local = true
	}

	return
}

func (e *TrackEngine) StoreLocalTrack() error {
	return StoreLocalTrack(e.Them.GetUid(), e.trackStatement)
}

func StoreLocalTrack(id UID, statement *jsonw.Wrapper) error {
	G.Log.Debug("| StoreLocalTrack")
	return G.LocalDb.Put(
		DbKey{Typ: DB_LOCAL_TRACK, Key: id.ToString()},
		nil,
		statement,
	)
}

func (e *TrackEngine) StoreRemoteTrack() (err error) {
	G.Log.Debug("+ StoreRemoteTrack")
	defer G.Log.Debug("- StoreRemoteTrack -> %s", ErrToOk(err))

	if e.signingKey, err = G.Keyrings.GetSecretKey("tracking signature"); err != nil {
		return
	} else if e.signingKey == nil {
		err = NoSecretKeyError{}
		return
	}

	if e.sig, e.sigid, err = SimpleSign(e.trackStatementBytes, *e.signingKey); err != nil {
		return
	}

	_, err = G.API.Post(ApiArg{
		Endpoint:    "follow",
		NeedSession: true,
		Args: HttpArgs{
			"sig_id_base":  S{e.sigid.ToString(false)},
			"sig_id_short": S{e.sigid.ToShortId()},
			"sig":          S{e.sig},
			"uid":          S{e.Them.GetUid().ToString()},
			"type":         S{"track"},
		},
	})

	return
}

//=====================================================================
