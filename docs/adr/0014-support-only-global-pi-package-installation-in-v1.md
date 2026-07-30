# Support only global Pi package installation in 0.1

Matty `0.1` certifies only a global package installation performed through Pi.
This gives the Matty User one consistent Core runtime across trusted
repositories without introducing a separate executable or a second certified
installation scope.

Pi may permit project-local package installation. Matty does not block that
host behavior or claim it cannot work, but it remains outside the `0.1` support
and test matrix because it gives a repository influence over executable package
loading and requires separate precedence, trust, and lifecycle certification.
