# Zealot
![Language](https://img.shields.io/github/languages/top/ANVF-Postal/Zealot?logo=go&logoColor=white)
![Made with spite](https://img.shields.io/badge/made%20with-spite-purple)
![Terminal gremlin](https://img.shields.io/badge/terminal-gremlin%20approved-39ff14)
![Works on my machine](https://img.shields.io/badge/works-on%20my%20machine-orange)

Zealot is a terminal based real-time messaging app designed to be contained on one machine and allow instant messaging between connected users. It does not use networking, and achieves Xkeyscore resistance through inter-process communication over a local daemon. It's not an IRC server and doesn't require any heavy installs, complicated setup or webservers. Just clone, compile and run.

Zealot was designed to solve a very specific problem of direct messaging between users logged in to a single server, where previous options were limited to older software like `write` and `mutt`. It was built to be flexible, feature-rich, intuitive and good looking. Special thanks to Lemunozm's [termchat](https://github.com/lemunozm/termchat) which directly inspired the creation of Zealot.

# Feature overview

-Full TUI based on BubbleTea

-Modern, intuitive interface that gives you an actual chat window

-Room based chatting, with unlimited rooms possible

-Messages history is optional, off by default and clears when all users leave a room

-Indicators for when a user is typing

-User join and leave messages

-Per-user settings file to customize Zealot

-Ephemeral messaging: Send terminal outputs straight to Zealot from command line

-Chat commands!

-Mouse support

-Built-in themes, editable custom themes, and shareable theme packs

-And more

# Creating theme packs

Open **[THEMES]** from Zealot's start screen. Built-in themes are read-only; clone one with **[CLONE]** or create a new custom theme with **[+]** to customize it.

To assemble a pack, select a custom theme and click **[PACK]** at the top right of its details pane. In pack mode, use **Space** or click names to mark additional custom themes with a **#**. Built-in themes appear muted and cannot be included. Press **Enter** or click **[CONFIRM]**, type a pack name in the composer, then press **Enter** to save. **Esc** cancels naming or exits pack mode without changing files.

Zealot writes the new `pack-<name>.toml` file to your themes directory. To avoid duplicate themes in the browser, selected presets are removed from their original live files, but the **original TOML files are preserved** in the `themes/packed/` archive directory, which Zealot does not load. If an original pack had unselected themes, those remain available in the live library. Themes in non-default packs remain editable.

# Usage warning ⚠️
Using Zealot for chatting will reduce your overall online footprint and make it harder for hackers or governments to spy on your messages. This may affect relationships with FBI, CIA, NSA, DOJ, CSIS, NATO and United Nations employees who are interested in wiretapping you. We are not responsible for any distress, face-palming, boring workdays, rolling eyes or any other side effects using Zealot may have on government workers. If you're feeling lonely and would prefer a chat app that spies on you, try [Discord!](https://discord.com)

# Xkeyscore resistance
Xkeyscore is a classified American analytical system that searches, analyzes and exploits extremely high volumes of global internet data that the NSA collects in real time from numerous upstream sources. These upstream sources include fiberoptic wiretaps installed in major telecommunications facilities (Like the ones found in [Room 641A](https://wikipedia.org/wiki/Room_641A)), PRISM (A legal framework that forces any tech company to provide information on its users), FAIRVIEW (a program where AT&T voluntarily collaborated with the NSA to collect international and domestic telecommunications) and more.

While Zealot is designed to reduce the usable data footprint of its messages, no internet communication can definitively avoid NSA wiretaps. True immunity to collection is impossible over the internet.

However, Zealot's architecture leaves limited usable data behind to be analyzed. Provided your SSH connection to the server is properly encrypted, Xkeyscore can only see and track certain metadata associated with internet packets sent over the SSH connection. Spy agencies can still technically track when you sent messages over SSH, what encryption methods were used and the volume of traffic engaged. This is a consequence of these agencies wiretapping vast portions of the internet.

Important reminder: If an attacker gains either remote or physical access to the system, all data is compromised. While zealot chats do not live forever, they can be intercepted and saved unencrypted if Zealot is used on a compromised system.

# AI Coded?
This project made extensive use of AI programming, primarily from ChatGPT. While many may scoff at this among all the vibe-coded apps we're seeing pop up everywhere, allow me to say this: I'm not a programmer. I could spend a year learning to code and then many weeks creating Zealot while constantly consulting google for bugs, dealing with compiler errors, or maybe just getting fed up and dropping the project entirely. AI graciously allows non-programmers to execute their ideas for software and give the results away to you. Zealot was human-designed, not prompted with "Write me a secure chat app. Make no mistakes". Everything from the way word-wrapping works, to the general UI layout, to the behaviors of buttons and other featured were designed by me, a human.
