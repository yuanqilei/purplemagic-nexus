package gmtool

import (
	"context"
	"dnf90.local/gateway/internal/modules/dnf/logincodes"
	"dnf90.local/gateway/internal/modules/dnf/repository"
	"dnf90.local/gateway/internal/modules/dnf/testbenefits"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) v1Gifts(w http.ResponseWriter, r *http.Request) {
	if s.deps.LoginCodes == nil {
		v1Fail(w, r, 503, "unavailable", "礼包服务不可用", true)
		return
	}
	if r.Method == "GET" {
		if r.URL.Query().Get("claims") == "1" {
			v, e := s.deps.LoginCodes.GiftClaims(r.Context())
			if e != nil {
				s.v1Unavailable(w, r, e)
				return
			}
			v1Reply(w, r, 200, v1Envelope{Data: v})
			return
		}
		v, e := s.deps.LoginCodes.Gifts(r.Context())
		if e != nil {
			s.v1Unavailable(w, r, e)
			return
		}
		v1Reply(w, r, 200, v1Envelope{Data: v})
		return
	}
	var in struct {
		Name     string                 `json:"name"`
		Campaign string                 `json:"campaign"`
		Count    int                    `json:"count"`
		Phrase   string                 `json:"phrase"`
		MaxUses  int                    `json:"maxUses"`
		StartsAt time.Time              `json:"startsAt"`
		EndsAt   time.Time              `json:"endsAt"`
		Draft    testbenefits.MailDraft `json:"draft"`
	}
	if !v1Decode(w, r, &in) {
		return
	}
	if in.Phrase == "" {
		in.MaxUses = 1
	}
	if in.StartsAt.IsZero() {
		in.StartsAt = time.Now().UTC()
	}
	if in.Campaign == "" {
		v1Fail(w, r, 400, "invalid_campaign", "请设置活动编号，同活动每账号限领一次", false)
		return
	}
	if !s.v1RequireItemIndex(w, r) {
		return
	}
	if in.Draft.Gold != 0 || in.Draft.ItemID == "" {
		v1Fail(w, r, 400, "invalid_reward", "请选择物品邮件奖励", false)
		return
	}
	in.Draft.PVFChecksum = s.v1PVFChecksum()
	if len(in.Draft.PVFChecksum) != 64 {
		v1Fail(w, r, 503, "pvf_unavailable", "当前奖励资源版本不可用，请先加载物品库", true)
		return
	}
	in.Draft.Title = in.Name
	in.Draft.Options = testbenefits.MailOptions{}
	reward, err := s.v1ProjectMailDraft(in.Draft, time.Now().UTC())
	if err != nil {
		s.v1OperationError(w, r, err)
		return
	}
	reward.PVFChecksum = in.Draft.PVFChecksum
	raw, err := json.Marshal(reward)
	if err != nil {
		s.v1Unavailable(w, r, err)
		return
	}
	codes, err := s.deps.LoginCodes.CreateGifts(r.Context(), logincodes.Gift{Name: in.Name, Campaign: in.Campaign, MaxUses: in.MaxUses, StartsAt: in.StartsAt, EndsAt: in.EndsAt, Reward: raw}, in.Count, in.Phrase)
	if err != nil {
		v1Fail(w, r, 400, "invalid_gift", "创建失败，请检查参数或口令是否重复", false)
		return
	}
	v1Reply(w, r, 200, v1Envelope{Data: map[string]any{"codes": codes}})
}
func (s *Server) v1DisableGift(w http.ResponseWriter, r *http.Request) {
	var in struct{}
	if !v1Decode(w, r, &in) {
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		v1Fail(w, r, 400, "invalid_id", "编号无效", false)
		return
	}
	if s.deps.LoginCodes == nil {
		v1Fail(w, r, 503, "unavailable", "礼包服务不可用", true)
		return
	}
	if err = s.deps.LoginCodes.DisableGift(r.Context(), id); err != nil {
		s.v1Unavailable(w, r, err)
		return
	}
	v1Reply(w, r, 200, v1Envelope{Data: map[string]bool{"ok": true}})
}

// PublicGiftHandler exposes only player-owned reads and redemption. It never
// accepts management credentials or an account ID supplied by the client.
func (s *Server) PublicGiftHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if s.deps.LoginCodes == nil {
			http.Error(w, "礼包服务不可用", 503)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		r = r.WithContext(ctx)
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			http.Error(w, "请重新登录", 401)
			return
		}
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		account, err := s.deps.LoginCodes.GiftAccount(r.Context(), token)
		if err != nil {
			http.Error(w, "请重新登录", 401)
			return
		}
		group, err := s.v1OperationGroup()
		if err != nil {
			http.Error(w, "礼包服务不可用", 503)
			return
		}
		if r.Method == "GET" && r.URL.Path == "/api/gifts/characters" {
			chars, e := group.Character.ListByAccount(r.Context(), account, 32)
			if e != nil {
				http.Error(w, "角色读取失败", 503)
				return
			}
			out := []map[string]string{}
			for _, c := range chars {
				if c.Stats["delete_flag"] == 0 {
					out = append(out, map[string]string{"id": c.CharacterID, "name": c.Name})
				}
			}
			v1Reply(w, r, 200, v1Envelope{Data: out})
			return
		}
		if r.Method != "POST" || r.URL.Path != "/api/gifts/redeem" {
			http.NotFound(w, r)
			return
		}
		var in struct {
			Code      string `json:"code"`
			Character string `json:"characterId"`
		}
		if !v1Decode(w, r, &in) {
			return
		}
		if len(in.Code) > 128 || len(in.Character) > 64 {
			http.Error(w, "兑换参数无效", 400)
			return
		}
		character, found, err := group.Character.Load(r.Context(), in.Character)
		if err != nil || !found || character.AccountID != account || character.Stats["delete_flag"] != 0 {
			http.Error(w, "请选择本账号角色", 403)
			return
		}
		g, claim, err := s.deps.LoginCodes.ReserveGift(r.Context(), in.Code, account, in.Character)
		if err != nil {
			http.Error(w, "礼包无效、已领取或已结束，请核对兑换记录", 409)
			return
		}
		if character.CreatedAt.After(claim.CreatedAt) {
			http.Error(w, "角色已重新创建，请联系管理员", 409)
			return
		}
		if claim.Status != "delivered" {
			var reward testbenefits.Job
			if json.Unmarshal(g.Reward, &reward) != nil {
				http.Error(w, "奖励配置不可用", 503)
				return
			}
			if reward.PVFChecksum != "" && reward.PVFChecksum != s.v1PVFChecksum() {
				http.Error(w, "奖励资源版本已变化，请联系管理员", 409)
				return
			}
			mail, _, e := sendItemMailOnce(r.Context(), group, character, "gift:"+claim.ID, claim.ID, repository.SystemMailDelivery{RecipientCharacterID: character.CharacterID, Title: g.Name, Body: reward.Body, Source: "gift:" + claim.ID, Attachments: reward.Attachments, CreatedAt: claim.CreatedAt})
			if e != nil {
				http.Error(w, "发放未完成，请用原码和原角色重试", 503)
				return
			}
			if e = s.deps.LoginCodes.FinishGift(r.Context(), claim.ID, mail); e != nil {
				http.Error(w, "回执待确认，请重试", 503)
				return
			}
			claim.Status = "delivered"
			claim.MailID = mail
		}
		v1Reply(w, r, 200, v1Envelope{Data: claim})
	})
}
