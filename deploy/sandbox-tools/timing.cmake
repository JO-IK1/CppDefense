# RULE_LAUNCH_* works with both Ninja and Makefile generators. It wraps the
# compiler launcher too, so compiler-cache lookup time is included.
set_property(GLOBAL PROPERTY RULE_LAUNCH_COMPILE "/opt/cppdefense/timed.sh compile")
set_property(GLOBAL PROPERTY RULE_LAUNCH_LINK "/opt/cppdefense/timed.sh link")
