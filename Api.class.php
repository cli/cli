<?php

namespace APITracking;

class Api
{
	
	# @var string The API key to be used for requests.
	public static $apiKey;
	
	# @var string The client id to be used for requests.
	public static $clientId;
	
	# @var string The base URL to be used for requests.
	public static $apiBaseUrl = "api.trackingmore.com";
	
	# @var string The port to be used for requests.
	public static $apiPort = 443;
	
	# @var string The version to be used for requests.
	public static $apiVersion = "v2";
	
	# @var string The path to be used for requests.
	public static $apiPath;
	
	# @var string The header key.
	public static $headerKey = "Trackingmore-Api-Key";
	
	# @var string The tracking number.
	public static $trackingNumber;
	
	# @var string The express.
	public static $trackingExpress;
	
	# @var string The lang.
	public static $trackingLang;
	
	# @var string The pattern express.
	public static $patternExpress = "/^[0-9a-z-_]+$/i";
	
	# @var string The pattern number.
	public static $patternNumber = "/^[0-9a-z-_]{5,}$/i";
	
	# @var string The pattern lang.
	public static $patternLang = "/^[a-z]{2}$/i";
	
	/**
	* @return string The complete url.
	*/
	public static function getBaseUrl($path = null)
	{
		$port = self::$apiPort == 443 ? "https" : "http";
		$url = $port . "://" . self::$apiBaseUrl . "/" . self::$apiVersion;
		if($path !== null) $url .= "/{$path}";
		return $url;
	}
	
	/**
	* Sets the API key to be used for requests.
	*
	* @param string $apiKey
	*/
	public static function setApiKey($apiKey)
	{
		self::$apiKey = $apiKey;
	}
   
	/**
	* Sets the client id to be used for requests.
	*
	* @param string $clientId
	*/
	public static function setClientId($clientId)
	{
		self::$clientId = $clientId;
	}
	
	/**
	* Check if the express meets the requirements.
	*
	* @param string $express.
	* @return boolean.
	*/
	public static function checkExpressRequirements()
	{
		if(empty(self::$trackingExpress) || !is_string(self::$trackingExpress)) return false;
		return preg_match(self::$patternExpress,self::$trackingExpress);
	}
	
	/**
	* Check if the tracking number meets the requirements.
	*
	* @return boolean.
	*/
	public static function checkNumberRequirements()
	{
		if(empty(self::$trackingNumber) || !is_string(self::$trackingNumber)) return false;
		return preg_match(self::$patternNumber,self::$trackingNumber);
	}
	
	/**
	* Check if the tracking number meets the requirements.
	*
	* @return boolean.
	*/
	public static function checkLangRequirements()
	{
		if(empty(self::$trackingLang) || !is_string(self::$trackingLang)) return false;
		return preg_match(self::$patternLang,self::$trackingLang);
	}
	
	/**
	* Check if the tracking number and express meets the requirements.
	*
	* @return boolean.
	*/
	public static function checkParamsRequirements($params)
	{
		self::$trackingNumber = empty($params["tracking_number"]) ? "" : trim($params["tracking_number"]);
		self::$trackingExpress = empty($params["carrier_code"]) ? "" : trim($params["carrier_code"]);
		self::$trackingLang = empty($params["lang"]) ? "" : trim($params["lang"]);
		if(!self::checkLangRequirements()) self::$trackingLang = "";
		if(!self::checkNumberRequirements()) return 4014;
		if(!self::checkExpressRequirements()) return 4015;
		return true;
	}
	
	/**
	* Check if the data meets the requirements.
	*
	* @return json response.
	*/
	public static function checkSendApi($data)
	{
		if(empty($data) || !is_array($data)) return self::errorResponse(4501);
		$params = [];
		foreach($data as $value)
		{
			$check = self::checkParamsRequirements($value);
			if($check !== true) continue;
			$params[] = $value;
		}
		if(empty($params)) return self::errorResponse(4501);
		return self::sendApiRequest($params);
	}
	
	/**
	* gets the header to be used for requests.
	*
	* @return array $header.
	*/
	public static function getRequestHeader()
	{
		$header = [
			"Content-Type: application/json",
			self::$headerKey.": " . self::$apiKey,
		];
		if(!empty(self::$clientId)) $header[] = "Client-Id: ". self::$clientId;
		return $header;
	}
	
	/**
	* send api request.
	*
	* @return array $response.
	*/
	public static function sendApiRequest($params = [], $method = "GET")
	{
		RequestApi::$url = self::getBaseUrl(static::$apiPath);
		RequestApi::$params = $params;
		RequestApi::$header = self::getRequestHeader();
		return RequestApi::send($method);
	}
	
	/**
	* error params request.
	*
	* @return json response.
	*/
	public static function errorResponse($code, $message = "", $data = [])
	{
		$errorMessage = [
			4014 => "The value of `tracking_number` is invalid.",
			4015 => "The value of `carrier_code` is invalid.",
			4501 => "The submitted parameters do not meet the requirements",
			4502 => "Signature verification failed",
			4503 => "No data received or data format error",
		];
		if(
			empty($message) 
			&& isset($errorMessage[$code])
		) $message = $errorMessage[$code];
		return json_encode([
			"meta" => [
				"code" => $code,
				"type" => "Error",
				"message" => $message,
			],
			"data" => $data,
		]);
	}
	
}