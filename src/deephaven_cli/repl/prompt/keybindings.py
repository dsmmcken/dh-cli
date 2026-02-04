"""Custom key bindings for Deephaven REPL."""
from prompt_toolkit.key_binding import KeyBindings
from prompt_toolkit.keys import Keys


def create_key_bindings() -> KeyBindings:
    """Create custom key bindings for the REPL."""
    bindings = KeyBindings()

    @bindings.add(Keys.Enter)
    def submit_on_enter(event):
        """Submit input on Enter."""
        event.current_buffer.validate_and_handle()

    @bindings.add("escape", "enter")
    def newline_on_alt_enter(event):
        """Insert newline on Alt+Enter (Escape+Enter)."""
        event.current_buffer.insert_text("\n")

    @bindings.add(Keys.ControlL)
    def clear_screen(event):
        """Clear the screen."""
        event.app.renderer.clear()

    return bindings
