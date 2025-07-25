The data in this directory consists of real certificates issued by Let's
Encrypt in 2023. The ones under the `bad` directory were issued during
the Duplicate Serial Numbers incident (https://bugzilla.mozilla.org/show_bug.cgi?id=1838667)
and differ in the presence / absence of a second policyIdentifier in the
Certificate Policies extension.

The ones under the `good` directory were issued shortly after recovery
from the incident and represent a correct correspondence relationship.
