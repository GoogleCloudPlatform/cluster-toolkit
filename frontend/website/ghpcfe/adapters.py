from allauth.account.adapter import DefaultAccountAdapter
from allauth.socialaccount.adapter import DefaultSocialAccountAdapter
from .models import AuthorisedUser

import logging
logger = logging.getLogger(__name__)

class CustomAccountAdapter(DefaultAccountAdapter):
    def is_open_for_signup(self, request):
        return False # No signups allowed

class CustomSocialAccountAdapter(DefaultSocialAccountAdapter):
    def is_open_for_signup(self, request, sociallogin):
        u = sociallogin.user
        ret = False
        authorised = AuthorisedUser.objects.all()
        for entry in authorised:
            if u.email == entry.pattern: # exact email address match
                ret = True
                break
            if entry.pattern in u.email: # domain name match
                ret = True
                break
        if ret:
            logger.info(f"User {u.email} logged in with Google account.")
        else:
            logger.info(f"User {u.email} not authorised to access this system.")
        return ret
