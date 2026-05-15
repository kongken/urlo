# urlo Roadmap Notes

`urlo` is already beyond a minimal URL shortener. The current codebase includes:

- short link creation with custom aliases and configurable code length
- optional Google login and per-user link listing
- link edit, delete, disable/enable, and expiration support
- QR code generation in the frontend
- click logging plus basic analytics for referrer, country, browser, device, and daily trend
- rate limiting and pluggable storage/click backends

That means the next step should not be "add more CRUD". The better direction is moving `urlo` from a personal utility into a product that teams can operate and trust.

## Priority 1: productize link management

These are the highest-value gaps because the current data model is still flat and single-user oriented.

### 1. Link organization

Add folders, tags, search filters, and archived views.

Why it matters:

- the dashboard already lists links, but there is no way to group campaigns, projects, or channels
- analytics become much more useful when links can be filtered by tag or collection
- this is a prerequisite for any serious team workflow

Suggested scope:

- add `tags` and optional `folder` fields to the link record
- extend list endpoints with filter params
- expose tag/folder management in dashboard

### 2. Stronger ownership and sharing model

Current ownership is a single Google `sub`, which is enough for "my links" but not for collaboration.

Why it matters:

- no shared workspace model
- no read-only analyst access
- no handoff when one person creates links for a team

Suggested scope:

- introduce organizations or workspaces
- support roles such as owner, editor, viewer
- move link ownership from single user to workspace or shared project

### 3. Safer anonymous-link behavior

Anonymous links are currently editable and deletable by code when unowned.

Why it matters:

- convenient for demos, risky for production use
- difficult to explain trust and authority boundaries to users

Suggested scope:

- add a config switch to disable anonymous management
- optionally issue a secret management token for anonymous links
- surface the security mode in UI copy

## Priority 2: make analytics genuinely useful

Analytics exists today, but it is still basic and mostly per-link.

### 4. UTM and campaign analytics

Why it matters:

- short links are often used for campaign tracking, not just redirection
- grouping by campaign/source/medium is more actionable than raw click counts

Suggested scope:

- parse and store UTM parameters from destination URLs
- allow manual campaign labels on link creation
- add campaign-level aggregation pages

### 5. Better geography and audience insights

Country and city fields exist, but GeoIP is not wired.

Why it matters:

- current UI already reserves space for location analytics
- this is a visible incomplete feature rather than a missing one

Suggested scope:

- plug in GeoIP enrichment
- add timezone and language breakdowns
- show top cities and regional trends

### 6. Export and webhook delivery

Why it matters:

- teams will want to pull click data into BI tools, spreadsheets, or bots
- dashboards are not enough for operational workflows

Suggested scope:

- CSV export for links and click logs
- webhook on click or threshold events
- scheduled digest email or webhook summaries

## Priority 3: improve distribution and growth use cases

### 7. Custom domains

Why it matters:

- this is one of the clearest steps from internal tool to customer-facing product
- branded domains improve trust and click-through

Suggested scope:

- multiple domains per workspace
- domain verification and health checks
- per-domain analytics and routing policies

### 8. Link routing rules

Why it matters:

- many modern shorteners support destination logic, not just static redirects
- useful for device-specific app deep links, A/B tests, and geo routing

Suggested scope:

- conditional destinations by country, device, or language
- expiration fallback pages
- percentage rollout / split testing

### 9. Bulk operations

Why it matters:

- campaign setup is slow if every link is manual
- necessary for imports, migrations, and marketing teams

Suggested scope:

- CSV import/export
- bulk create with shared prefix/tag/campaign metadata
- bulk enable, disable, retarget, or delete

## Priority 4: operational maturity

### 10. Abuse prevention and review flows

Why it matters:

- link shorteners are attractive targets for spam and phishing
- current rate limiting helps, but it does not cover reputation or review workflows

Suggested scope:

- domain allowlist/blocklist
- suspicious destination scanning
- moderation queue and audit log

### 11. API tokens and service integrations

Why it matters:

- Google session login is fine for humans, not enough for automation
- product teams will want CI, scripts, bots, and external apps

Suggested scope:

- personal access tokens or workspace API keys
- scoped permissions
- token usage logs and revocation UI

### 12. Observability and admin controls

Why it matters:

- once traffic grows, operators need visibility beyond per-link pages
- frontend already exposes user-facing analytics, but admins need service-level views

Suggested scope:

- admin dashboard for total traffic, top domains, errors, and abuse flags
- queue depth / recorder health / storage latency metrics
- retention settings for click logs

## Recommended development order

If only a few items should be built next, the best sequence is:

1. link organization
2. safer anonymous-link management
3. custom domains
4. UTM / campaign analytics
5. API tokens
6. CSV export and bulk operations

This order matches the current architecture: it extends the existing link, owner, and analytics model without forcing a full rewrite first.
