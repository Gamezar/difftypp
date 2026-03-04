/**
 * Global test setup for jsdom environment.
 * Stubs browser APIs that jsdom doesn't implement.
 */

// scrollIntoView is not implemented in jsdom
Element.prototype.scrollIntoView = function () {}
