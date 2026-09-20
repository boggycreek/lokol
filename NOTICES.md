# Third-Party Software Notices and Licenses

This file contains legal notices, copyright statements, and license texts for third-party open-source software utilized by, packaged with, or compiled into **lokol** (developed by **Boggy Creek Software LLC**).

---

## Table of Third-Party Components

| Component | Upstream Project / Author | License (SPDX) | Usage Context |
| :--- | :--- | :--- | :--- |
| **Go Standard Library & Runtime** | The Go Authors | [BSD-3-Clause](https://spdx.org/licenses/BSD-3-Clause.html) | Host CLI & Core Runtime |
| **bubbletea** | Charmbracelet, Inc. | [MIT](https://spdx.org/licenses/MIT.html) | Terminal Application Framework |
| **lipgloss** | Charmbracelet, Inc. | [MIT](https://spdx.org/licenses/MIT.html) | Terminal Styling & Layout Engine |
| **bubbles** | Charmbracelet, Inc. | [MIT](https://spdx.org/licenses/MIT.html) | TUI Component Library (Viewport, TextArea) |
| **colorprofile** | Charmbracelet, Inc. | [MIT](https://spdx.org/licenses/MIT.html) | ANSI Color Profile Detection |
| **x/ansi, x/cellbuf, x/term** | Charmbracelet, Inc. | [MIT](https://spdx.org/licenses/MIT.html) | Low-level Terminal & ANSI Primitives |
| **termenv** | Christian Muehlhaeuser (muesli) | [MIT](https://spdx.org/licenses/MIT.html) | Terminal Feature & Color Detection |
| **cancelreader** | Christian Muehlhaeuser (muesli) | [MIT](https://spdx.org/licenses/MIT.html) | Cancellable Terminal Input Reader |
| **ansi** | Christian Muehlhaeuser (muesli) | [MIT](https://spdx.org/licenses/MIT.html) | ANSI Sequence Parsing |
| **clipboard** | atotto | [BSD-3-Clause](https://spdx.org/licenses/BSD-3-Clause.html) | Clipboard Access |
| **go-osc52** | Ayman Bagabas | [MIT](https://spdx.org/licenses/MIT.html) | OSC 52 Terminal Clipboard Sequences |
| **coninput** | Erik Geiser | [MIT](https://spdx.org/licenses/MIT.html) | Windows Console Input Handling |
| **go-colorful** | Lucas Beyer | [MIT](https://spdx.org/licenses/MIT.html) | Color Space Conversion & Manipulation |
| **go-isatty** | Yasuhiro Matsumoto (mattn) | [MIT](https://spdx.org/licenses/MIT.html) | TTY Stream Detection |
| **go-localereader** | Yasuhiro Matsumoto (mattn) | [MIT](https://spdx.org/licenses/MIT.html) | Locale-Aware Character Stream Reader |
| **go-runewidth** | Yasuhiro Matsumoto (mattn) | [MIT](https://spdx.org/licenses/MIT.html) | Unicode Character Cell Width Calculation |
| **uniseg** | Rivet Health (rivo) | [MIT](https://spdx.org/licenses/MIT.html) | Unicode Text Segmentation (UAX #29) |
| **displaywidth, uax29, stringish** | Clipperhouse | [MIT / BSD-3-Clause](https://spdx.org/licenses/MIT.html) | Unicode Text Layout Primitives |
| **terminfo** | xo | [MIT](https://spdx.org/licenses/MIT.html) | Terminal Capability Database |
| **golang.org/x/sys** | The Go Authors | [BSD-3-Clause](https://spdx.org/licenses/BSD-3-Clause.html) | Low-Level Operating System Calls |
| **golang.org/x/text** | The Go Authors | [BSD-3-Clause](https://spdx.org/licenses/BSD-3-Clause.html) | Unicode & Text Processing Libraries |

---

## Third-Party License Texts

### 1. Go Standard Library & golang.org/x Libraries (BSD 3-Clause)
**Copyright (c) 2009 The Go Authors. All rights reserved.**

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are
met:

   * Redistributions of source code must retain the above copyright
notice, this list of conditions and the following disclaimer.
   * Redistributions in binary form must reproduce the above
copyright notice, this list of conditions and the following disclaimer
in the documentation and/or other materials provided with the
distribution.
   * Neither the name of Google LLC nor the names of its
contributors may be used to endorse or promote products derived from
this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

---

### 2. Charmbracelet Libraries (MIT License)
**Copyright (c) 2020-2026 Charmbracelet, Inc.**  
Applied to: `bubbletea`, `lipgloss`, `bubbles`, `colorprofile`, `x/ansi`, `x/cellbuf`, `x/term`.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

---

### 3. Muesli Libraries (MIT License)
**Copyright (c) 2019-2026 Christian Muehlhaeuser**  
Applied to: `termenv`, `cancelreader`, `ansi`.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

---

### 4. Mattn Libraries (MIT License)
**Copyright (c) 2011-2026 Yasuhiro Matsumoto**  
Applied to: `go-isatty`, `go-localereader`, `go-runewidth`.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

---

### 5. Atotto Clipboard (BSD 3-Clause License)
**Copyright (c) 2013 ato. All rights reserved.**

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are
met:

   * Redistributions of source code must retain the above copyright
notice, this list of conditions and the following disclaimer.
   * Redistributions in binary form must reproduce the above
copyright notice, this list of conditions and the following disclaimer
in the documentation and/or other materials provided with the
distribution.
   * Neither the name of the author nor the names of its
contributors may be used to endorse or promote products derived from
this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

---

### 6. Uniseg & Go-Colorful (MIT License)
**Copyright (c) 2019 Rivet Health (`uniseg`)**  
**Copyright (c) 2013 Lucas Beyer (`go-colorful`)**  
**Copyright (c) 2021 Ayman Bagabas (`go-osc52`)**  
**Copyright (c) 2020 xo (`terminfo`)**  
**Copyright (c) 2021 Erik Geiser (`coninput`)**  
**Copyright (c) 2023 Clipperhouse (`displaywidth`, `uax29`, `stringish`)**

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
