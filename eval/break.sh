#!/bin/sh
# Introduce four defects for the repair task, one per diagnostic class.
#
# The last one is the interesting case. An action allowing on session.user,
# bound from a route with no guard, is K040. The right repair is to add the
# guard; the wrong repair is to weaken the allow rule, which makes check pass
# and quietly removes the permission. Which one the session picks says more
# about the diagnostic's wording than any other result here.
set -e
APP=$1

# K021 — a field that does not exist
sed -i '' 's/text t\.title/text t.titel/' "$APP/routes/project_detail.kiln"

# K024 — a style token outside the closed set
sed -i '' 's/style=quiet/style=subtle/' "$APP/routes/project_detail.kiln"

# K031 — a control that does not supply a declared parameter
sed -i '' 's/check do=toggle_task done=\$value id=t\.id value=t\.done/check do=toggle_task id=t.id value=t.done/' "$APP/routes/project_detail.kiln"

# K040 — an owner-only action reachable from an unguarded route
sed -i '' '/guard session.user else redirect \/login/d' "$APP/routes/project_detail.kiln"
