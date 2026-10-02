# Give a friend their own naknak behind Authentik (run inside `ak shell`).
#
#   docker compose exec -T -e FRIEND=max -e FRIEND_USER=max@example.org \
#     -e NAKNAK_DOMAIN=naknak.example.org -e TEMPLATE_PROVIDER=naknak \
#     -e OUTPOST=naknak-glaedr server ak shell < authentik-friend.py
#
# Copies flows and settings from TEMPLATE_PROVIDER (your own naknak), creates
# group naknak-<friend> (+ the user, if FRIEND_USER exists), a proxy provider
# for https://<friend>.<domain> → http://naknak-<friend>:8080, the application
# and its binding, and adds the provider to OUTPOST. Safe to run again.
# REMOVE=1 takes the friend's provider, application and group away again.
import os

from authentik.core.models import Application, Group, User
from authentik.outposts.models import Outpost
from authentik.policies.models import PolicyBinding
from authentik.providers.proxy.models import ProxyProvider

friend = os.environ["FRIEND"]
domain = os.environ["NAKNAK_DOMAIN"]
template = ProxyProvider.objects.get(name=os.environ.get("TEMPLATE_PROVIDER", "naknak"))
outpost = Outpost.objects.get(name=os.environ["OUTPOST"])
name = f"naknak-{friend}"

if os.environ.get("REMOVE") == "1":
    p = ProxyProvider.objects.filter(name=name).first()
    if p:
        outpost.providers.remove(p)
        Application.objects.filter(provider=p).delete()
        p.delete()
    Group.objects.filter(name=name).delete()
    print("RESULT removed", name)
else:
    group, _ = Group.objects.get_or_create(name=name)
    if os.environ.get("FRIEND_USER"):
        user = User.objects.filter(username=os.environ["FRIEND_USER"]).first() or User.objects.filter(email=os.environ["FRIEND_USER"]).first()
        if user:
            group.users.add(user)
        else:
            print("RESULT warning: no Authentik user", os.environ["FRIEND_USER"], "- add them to group", name, "later")
    p, _ = ProxyProvider.objects.update_or_create(name=name, defaults=dict(
        mode=template.mode,
        external_host=f"https://{friend}.{domain}",
        internal_host=f"http://{name}:8080",
        internal_host_ssl_validation=False,
        skip_path_regex=template.skip_path_regex,
        authorization_flow=template.authorization_flow,
        invalidation_flow=template.invalidation_flow,
        intercept_header_auth=template.intercept_header_auth,
        basic_auth_enabled=False,
        access_token_validity="days=30",  # a friend should not log in every hour
    ))
    p.set_oauth_defaults()  # without it Authentik shows "Redirect URI Error"
    p.save()
    app, _ = Application.objects.update_or_create(slug=name, defaults=dict(name=f"naknak · {friend}", provider=p, meta_launch_url=f"https://{friend}.{domain}"))
    PolicyBinding.objects.get_or_create(target=app, group=group, defaults=dict(order=0))
    outpost.providers.add(p)
    outpost.save()
    print("RESULT ok", name, p.external_host, "members:", [u.username for u in group.users.all()])
