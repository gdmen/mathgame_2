import React, { useState } from "react";

import './setup.scss'

/*

1. Sign up with Auth0
2. options.pin is -1 by default
3. set operations for your kid
3. add at least 3 videos your kid will like - validate youtube videos / make sure they load and play
4. set options.pin to chosen pin (repeat to confirm)
5. PIN IN MEMORY - if goto setup or parent page, keep pin. If goto any other page, purge pin
6. START PLAYING NOW (goto play) vs CONFIGURE THIS FOR YOUR KID! (goto parent/options page)
*/

const OperationsTabView = ({ token, url, user, options, postOptions, advanceSetup }) => {
  const allOperationsMap = new Map([
    ["Addition", "+"],
    ["Subtraction", "-"],
  ]);
  const revAllOperationsMap = new Map([
    ["+", "Addition"],
    ["-", "Subtraction"],
  ]);
  const [error, setError] = useState(false);
  const [operations, setOperations] = useState(options.operations.split(",").map(function(op, i) { return revAllOperationsMap.get(op); }));

  const handleCheckboxChange = (e) => {
    let id = e.target.id;
    let newArray = [...operations, id];
    if (operations.includes(id)) {
      newArray = operations.filter(x => x !== id);
    }
    setOperations(newArray);
    setError(newArray.length < 1);
  };

  const handleSubmitClick = (e) => {
    // post updated options
    options.operations = operations.map(function(op, i) {
      return allOperationsMap.get(op);
    }).join(",");
    postOptions(options);
    // redirect to next setup step
    advanceSetup();
  };

  return (<>
    <h2>Hi there! Let's do a little setup for your kid!</h2>
    <div className="setup-form">
      <h4>Which types of problems should we show? <span className={error ? "error" : ""}>Select one or more.</span></h4>
      <ul>
        {[...allOperationsMap.keys()].map(function(op, i) {
            var id = op;
            return (<li key={id}>
              <input type="checkbox" id={id} onChange={handleCheckboxChange} checked={"checked" ? operations.includes(id) : ""}/>
              <label htmlFor={id}>
                <div className="operation-button">
                  <span>{op}</span>
                </div>
              </label>
            </li>)
        })}
      </ul>
      <button className={error ? "error" : ""} onClick={handleSubmitClick}>continue</button>
    </div>
  </>)
}

const SetupView = ({ token, url, user, options }) => {
  const [activeTab, setActiveTab] = useState(null);

  //const allTabs = ["set operations", "add videos", "set parent pin", "start playing!"];
  const allTabs = ["Choose Operations", "Add Videos", "Set Parent Pin", "Start Playing!"];

  if (activeTab == null) {
    setActiveTab("Choose Operations");
  }

  const advanceSetup = function() {
    setActiveTab(allTabs[allTabs.indexOf(activeTab)+1]);
  }

  const handleTabClick = (e) => {
    let clickedId = parseInt(e.target.id.slice(-1));
    if (clickedId > allTabs.indexOf(activeTab)) {
      return;
    }
    setActiveTab(allTabs[clickedId]);
  }

  const postOptions = async function(options) {
      try {
        const settings = {
            method: 'POST',
            headers: {
              'Accept': 'application/json',
              'Content-Type': 'application/json',
              'Authorization': 'Bearer ' + token,
            },
            body: JSON.stringify(options),
        };
        const req = await fetch(url + "/options/" + options.user_id, settings);
        const json = await req.json();
        return json;
      } catch (e) {
        console.log(e.message);
      }
  };

  return (<div id="setup">
    <div id="setup-tabs">
      {allTabs.map(function(tab, i){
        var id = "tab" + i;
        var className = tab === activeTab ? "tab active" : "tab";
        return (
          <div key={id} className={className}>
            <div id={id} className="click-catcher" onClick={handleTabClick}></div>
            <span className="number">{i+1}</span>
            <span className="label">{tab}</span>
          </div>
        )
      })}
    </div>
    { (activeTab === "Choose Operations") && <div className="tabContent"><OperationsTabView token={token} url={url} user={user} options={options} postOptions={postOptions} advanceSetup={advanceSetup}/></div> }
  </div>)
}

export {
  SetupView
}
