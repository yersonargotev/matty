# Support only global Pi package installation in v1

Matty v1 certifies only a global package installation performed through Pi.
This gives the Matty User one consistent Shared Skill Catalog and runtime
across repositories without introducing a separate Matty executable.

Pi may also permit project-local package installation. Matty does not block
that host behavior, rewrite project configuration, or claim that such an
installation cannot work. It remains outside the v1 support and test matrix
because it gives a repository influence over loading an executable package and
creates a second installation scope whose precedence and lifecycle would need
separate certification.
