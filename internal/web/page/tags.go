package page

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/httproute"
	"medikube/internal/i18n"
	tagsvc "medikube/internal/service/tag"
	"medikube/internal/web"
	"medikube/internal/web/api"
	"medikube/internal/web/views/shell"
	viewtags "medikube/internal/web/views/tags"
)

// OpTagsPage is contracts/pages.md's /tags.
const OpTagsPage = "tagsPage"

const tagsListTitleID = "page.tagsPage.title"

// TagHandlers is /tags' contribution to the route table.
func TagHandlers(resolve api.TagResolve) (httproute.Handlers, error) {
	if resolve == nil {
		return nil, api.ErrNoTags
	}

	links, err := newTagLinks()
	if err != nil {
		return nil, err
	}

	pages := &tagPages{resolve: resolve, links: links}

	return httproute.Handlers{
		OpTagsPage: web.WithActor(pages.list),
	}, nil
}

type tagPages struct {
	resolve api.TagResolve
	links   tagLinks
}

func (p *tagPages) list(e *core.RequestEvent, actor access.Actor) error {
	if !actor.Authenticated() {
		return fmt.Errorf("page: the page needs a session: %w", domain.ErrForbidden)
	}

	service, err := p.resolve()
	if err != nil {
		return err
	}

	views, err := p.views(e, actor, service)
	if err != nil {
		return err
	}

	main := viewtags.Manager(viewtags.ManagerProps{
		Tags:       views,
		CreateHref: p.links.collection,
	})

	web.Localize(e)

	return RenderPage(e, http.StatusOK, i18n.T(e.Request.Context(), tagsListTitleID),
		NavState{SignedIn: true, Nav: p.links.nav(e.Request.Context(), e.Request.URL.Path)}, main)
}

func (p *tagPages) views(e *core.RequestEvent, actor access.Actor, service *tagsvc.Service) ([]viewtags.TagView, error) {
	page, err := service.List(e.Request.Context(), actor, tagsvc.Query{Limit: web.DefaultLimit})
	if err != nil {
		return nil, web.OwnerScoped(err)
	}

	ids := make([]string, 0, len(page.Items))
	for _, t := range page.Items {
		ids = append(ids, t.ID)
	}

	usage, err := service.Usage(e.Request.Context(), actor, ids)
	if err != nil {
		return nil, web.OwnerScoped(err)
	}

	views := make([]viewtags.TagView, 0, len(page.Items))
	for _, t := range page.Items {
		views = append(views, viewtags.NewTagView(t, usage[t.ID], p.links.of(t.ID)))
	}

	return views, nil
}

type tagLinks struct {
	listPage       string
	settingsPage   string
	medicationsURL string
	collection     string
	recordTmpl     string
}

func newTagLinks() (tagLinks, error) {
	paths, err := routePaths(map[string]string{
		OpTagsPage:           "",
		OpSettingsPage:       "",
		OpMedicationListPage: "",
		api.OpCreateTag:      "",
		api.OpUpdateTag:      "",
	})
	if err != nil {
		return tagLinks{}, err
	}

	return tagLinks{
		listPage:       paths[OpTagsPage],
		settingsPage:   paths[OpSettingsPage],
		medicationsURL: paths[OpMedicationListPage],
		collection:     paths[api.OpCreateTag],
		recordTmpl:     paths[api.OpUpdateTag],
	}, nil
}

func (l tagLinks) of(id string) viewtags.Links {
	if id == "" {
		return viewtags.Links{}
	}

	return viewtags.Links{Record: strings.ReplaceAll(l.recordTmpl, "{"+api.PathID+"}", id)}
}

func (l tagLinks) nav(ctx context.Context, current string) []shell.NavLink {
	return []shell.NavLink{
		{Label: i18n.T(ctx, "nav.medications"), Href: l.medicationsURL, Current: strings.HasPrefix(current, l.medicationsURL)},
		{Label: tagsListTitle, Href: l.listPage, Current: strings.HasPrefix(current, l.listPage)},
		{Label: i18n.T(ctx, "nav.settings"), Href: l.settingsPage, Current: current == l.settingsPage},
	}
}
