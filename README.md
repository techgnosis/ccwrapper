use your claude subscription better
lets make a harness for coding CLIs. It's a custom agent that really just uses top 3 agents in CLI  instead of chat mode.

There is no interaction, even questions. All it does it run the agent CLI in as verbose as possible with JSON to stdout. Then it presents the data in a very palatable format.

######### IMPORTANT
Because it's difficult to run claude in claude, just use @example.json as an example of CLI output
Do NOT try to run claude under any circumstance
Use the Anthropic API docs to fill in gaps missing from example.json. If you need me to run claude to create some scenario in example.json, ask me.
#########




# Outputs

Each input has the responses indented beneath it. Each input + indented outputs is a group. A group has a space between the other groups for visual ease.

Each tool use should show tool output indented beneath it

Colors can be used. I'm not sure how yet. Anything to make it very readable. Whatever successful tools do, or ease of readability studies.

# Dependencies
HTML5 + minimal CSS frameworks to make the output look great + vanilla JS, no frameworks of any kind

# Architecture
We'll use Golang so the claude CLI gets launched in a goroutine. Output captured and processed in real time.


# Package
Let's make it a super lean web app, assuming a web app can be packaged easily into a static Golang executable

Lets make it the simplest possible design. I really just need an input text field and then a big nice looking output on the rest of the screen that displays the contents of a running claude.

Only one instance of the agent at a time. Let it run its own agents.

Lets let each grouping only be 2 lines until it gets clicked on, then it expands rapidly. This will let lots of groups exist on the screen at once.

Let the user click on a question response to enter question answers and send.

Show a tab called Context that shows the context. Clear it any time

# Filesytem
Part of the web UI should show the file contents of ~/.claude and list ~/claude.json*

# Flags
The claude CLI will be run with --output-format=stream-json and --verbose
