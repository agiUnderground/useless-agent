# useless-agent 

![](/assets/img/demo2.png)

**What can this agent do?** Basically nothing, that’s why it is the useless-agent.  

**Why is it interesting?**  

* Uses text-only LLMs.  
* Cheap: I spent about $4.57 playing with it for about 7 evenings.  
* Single binary. *(almost, see todo list)*  
* Easy to use: run the binary and copy the IP address.  
* No telemetry, no bullshit.
* IPv4 & IPv6(*should work, not tested yet)

**:star: Would you like me to work more on this project? Please consider giving this repository a star!**  

> [!CAUTION]  
> * Only use this on a disposable virtual machine.  
> * It can, and most likely will, destroy your system.  
> * The LLM API provider has a realistic ability to inject malicious commands/actions/data into the ingested API responses.  
> * The video is not compressed. If you are connected to a virtual machine in the cloud, be aware of high internet traffic.  
> * The video stream and everything else, except the API queries, are not encrypted. If connecting to a remote machine, use an SSH tunnel with port forwarding.  

> [!NOTE]  
> It is super slow. Right now, speed is not a priority. If your only problem is speed, you have already won the agents game.  

Currently supported models:
* `deepseek-chat`
* `deepseek-reasoner`

**Environment**: Only works on - `Linux + xfce + X11`.  

Version - `v0.0.1`.  


# Changelog

### v0.0.1

* Added basic tasks management and status indication.
* Hotkeys support.
* UI update.
* Video stream FPS selector.

![](/assets/img/tasks-management.png)

# Q&A
* Can I put something like `http://127.0.0.1:11434/v1` into `API_BASE_URL=''` and use a model that I run locally? - Yes.

# Demos

`Prompt`: Open a web browser and go to deepseek.com

https://github.com/user-attachments/assets/6299f1b5-3c2e-493e-b67a-145487e14ffa

`Prompt`: Very detailed prompt to create a ping pong tcp client and server(neovim)

https://github.com/user-attachments/assets/370bdd7e-0955-4b1e-8e81-8ee3eb913e90



### How to build:
* `git clone`
* `cd useless-agent`
* `go build`

### How to prepare headless vm:
Scripts - "assets/scripts"
```bash
sudo apt install xfce4 xvfb tesseract-ocr-eng tesseract-ocr libtesseract5 libleptonica-dev libtesseract-dev
sudo apt remove xfce4-screensaver
sudo systemctl enable xvfb.service
sudo systemctl enable xfce4.service
sudo systemctl start xvfb.service
sudo systemctl start xfce4.service
```

### How to use:
`copy executable to the target machine`

`start executable:`
```bash
API_KEY=your-api-token-value DISPLAY=:1 API_BASE_URL='https://api.deepseek.com/v1' MODEL_ID='deepseek-chat' ./useless-agent --ip=127.0.0.1 --port 8080
```
`On client machine open main.html in the browser.`

`Put target machine IP into the field "IP Address".`

`Click "Connect".`

`Give it some task, for example "Open web browser", put that prompt into the LLM Chat and press "Send".`

> [!TIP]  
> Like to burn money? Try more capable LLMs; using DeepSeek R1 instead of v3 would probably make the program more capable of doing nothing.

How the project started: I just wanted to take a screenshot over the network.

### Todo list:  
- [ ] Build a single fully static binary.  
- [ ] Add stats about burned tokens per session/task.
- [ ] Allow LLM to spawn local 'thoughts', which would do/monitor something and then allow them to interrupt the main LOOP and to inject its results into the thinking loop.
- [ ] Build a unified concept space for models that are capable of ingesting more data.
- [ ] Pause task execution.
- [ ] Allow intervention in the execution process and provide guidance/additional instructions.

**Problems & Ideas:**  
* LLMs' context window/input size is so limiting; I want to shuffle in 100M million tokens at each iteration.  
* How to do reliable OCR? Don’t do it at all. The idea was to find a single point where Linux renders fonts for the whole system and intercept it, to set something like an eBPF hook and get all text for free. `Turns out there is no single point of font rendering in Linux!` Each X11 client app is allowed to render fonts itself. Nice.  
* How hard could it be to write a single function to recognize windows? They have unified patterns, right? `Wrong. Each window is allowed to draw its own header however they like.` I’m looking at you, Firefox. Nice.  
