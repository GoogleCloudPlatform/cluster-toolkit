# Copyright 2026 "Google LLC"
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Helpers shared by the identity modes."""


def default_oslogin_username(email):
    # Google's default OS Login username transform for an email identity:
    # "user.name@example.com" -> "user_name_example_com".
    return email.replace("@", "_").replace(".", "_")


def identity(email="", login_uid="", username_hint=None):
    """The single shape every mode returns.

    email        stable key for the session record
    login_uid    Google numeric subject, the authoritative OS Login join
    username_hint POSIX name to fall back on when no subject is available
    """
    return {
        "email": email,
        "login_uid": login_uid,
        "username_hint": username_hint,
    }
