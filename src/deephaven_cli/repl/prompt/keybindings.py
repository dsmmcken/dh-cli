"""Custom key bindings for Deephaven REPL."""
from prompt_toolkit.key_binding import KeyBindings
from prompt_toolkit.keys import Keys


def create_key_bindings() -> KeyBindings:
    """Create custom key bindings for the REPL."""
    bindings = KeyBindings()

    @bindings.add(Keys.ControlL)
    def clear_screen(event):
        """Clear the screen."""
        event.app.renderer.clear()

    return bindings
