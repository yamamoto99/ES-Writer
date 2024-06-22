import React, { useEffect } from "react";

import { useNavigate } from "react-router-dom";
import { useStorage } from "@plasmohq/storage/hook"

import genAnswer from "./genAnswer";
import openProfileForm from "./openProfileForm";

function IndexPopup() {
  const navigate = useNavigate();

  const [loginState, setLoginState] = useStorage<string>("loginState");

  // /welcomeをたたいて、成功すれば、ログイン済みとして扱う
  fetch("http://localhost:8080/welcome", {
    method: "GET"
  }).then((response) => {
    if (response.ok) {
      setLoginState("logged-in");
    } else {
      if (typeof loginState === undefined || loginState === "not-logged-in" || loginState === "logged-in"){
        setLoginState("not-logged-in");
      }
    }
  });

  if (loginState === "not-logged-in") {
    return (
      <div style={{ width: '150px', height: '75px' }}>
        <button onClick={() => {setLoginState("signUp");navigate("/signUp")}}>サインアップ</button>
        <button onClick={() => {setLoginState("signIn");navigate("/signIn")}}>サインイン</button>
      </div>
    );
  } else if (loginState === "logged-in") {
    return (
      <div style={{ width: '150px', height: '75px' }}>
        <button onClick={genAnswer}>回答生成</button>
        <button onClick={openProfileForm}>経歴入力</button>
      </div>
    );
  }else if (loginState === "signUp") {
    navigate("/signUp");
  }else if (loginState === "signIn") {
    navigate("/signIn");
  }else if (loginState === "checkEmail") {
    navigate("/checkEmail");
  }

}

export default IndexPopup;
