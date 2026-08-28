// Package di declares the container that assembles MediKube's own services.
//
// It is reached from the composition root and nowhere else: a package that
// resolves its own dependencies out of a container has hidden them.
package di
